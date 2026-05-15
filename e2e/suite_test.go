package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"text/template"
	"time"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	apiserverproc "github.com/alan-ghelardi/yaghan/e2e/internal/apiserver"
	daemonproc "github.com/alan-ghelardi/yaghan/e2e/internal/daemon"
	"github.com/alan-ghelardi/yaghan/e2e/internal/infra"
	"github.com/alan-ghelardi/yaghan/e2e/internal/yag"

	. "github.com/onsi/ginkgo/v2" //nolint:revive // ginkgo's idiom requires dot-import
	. "github.com/onsi/gomega"    //nolint:revive // gomega's idiom requires dot-import
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	apiServerProject = "yaghan-e2e-apiserver"
	daemonProject    = "yaghan-e2e-daemon"

	apiServerGRPCAddr = "localhost:9090"
	daemonGRPCAddr    = "localhost:9091"
	dynamoDBEndpoint  = "http://localhost:8000"
	redisAddr         = "localhost:6379"
	s3Endpoint        = "http://localhost:9000"

	snapshotBucket = "microvm-snapshots"

	bringupBudget = 2 * time.Minute
)

// SuiteContext is populated by BeforeSuite and consumed by individual
// specs. Anything a scenario needs to read (start time, the gRPC
// client connection to the api-server, …) hangs off here.
type SuiteContext struct {
	StartTime   time.Time
	APIServerCC *grpc.ClientConn
	ClusterCli  controlplanev1alpha1.ClusterServiceClient
	SandboxCli  controlplanev1alpha1.SandboxServiceClient
	RepoRoot    string
	RunDir      string
	ConfigDir   string
	AssetsDir   string

	// DaemonNodeID is the UUID the daemon-under-test reported when it
	// registered. Specs that issue ListSandboxes use it to scope the
	// query to "our" daemon — DynamoDB Local is shared, so a leftover
	// row from a previous run could otherwise leak through.
	DaemonNodeID string

	// EgressEnabled reflects what the daemon config was rendered
	// with. The network scenario reads this to decide whether to run.
	EgressEnabled bool
}

// suite is the package-level handle scenarios read from.
var suite SuiteContext

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	suite.StartTime = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), bringupBudget)
	DeferCleanup(cancel)

	suite.RepoRoot = mustRepoRoot()
	suite.AssetsDir = filepath.Join(suite.RepoRoot, "assets")
	suite.RunDir = mustRunDir(suite.RepoRoot)
	suite.ConfigDir = filepath.Join(suite.RunDir, "config")
	Expect(os.MkdirAll(suite.ConfigDir, 0o755)).To(Succeed())

	preflight(suite.AssetsDir, filepath.Join(suite.RepoRoot, "e2e", "bin"))

	GinkgoLogr.Info("e2e bringup", "runDir", suite.RunDir)
	AddReportEntry("e2e run dir", suite.RunDir)

	// --- compose stacks ------------------------------------------------
	apiCompose := filepath.Join(suite.RepoRoot, "api-server", "dev", "docker-compose.yml")
	daemonCompose := filepath.Join(suite.RepoRoot, "daemon", "dev", "docker-compose.yml")

	Expect(infra.Up(ctx, apiCompose, apiServerProject)).To(Succeed())
	DeferCleanup(func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer downCancel()
		_ = infra.Down(downCtx, apiCompose, apiServerProject)
	})
	Expect(infra.Up(ctx, daemonCompose, daemonProject)).To(Succeed())
	DeferCleanup(func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer downCancel()
		_ = infra.Down(downCtx, daemonCompose, daemonProject)
	})

	// --- backing service readiness ------------------------------------
	ddbClient, err := infra.NewDynamoDBClient(ctx, dynamoDBEndpoint, infra.DynamoDBLocalCreds)
	Expect(err).NotTo(HaveOccurred())
	s3Client, err := infra.NewS3Client(ctx, s3Endpoint, infra.MinIOCreds)
	Expect(err).NotTo(HaveOccurred())

	Expect(infra.WaitForDynamoDB(ctx, ddbClient)).To(Succeed())
	Expect(infra.WaitForRedis(ctx, redisAddr)).To(Succeed())
	Expect(infra.WaitForS3(ctx, s3Client)).To(Succeed())

	// --- DynamoDB tables (reset for a clean run) ----------------------
	tablesDir := filepath.Join(suite.RepoRoot, "api-server", "dynamodb-tables")
	entries, err := os.ReadDir(tablesDir)
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		Expect(infra.ResetTable(ctx, ddbClient, filepath.Join(tablesDir, entry.Name()))).
			To(Succeed())
	}

	// --- snapshot bucket -----------------------------------------------
	Expect(infra.EnsureBucket(ctx, s3Client, snapshotBucket)).To(Succeed())
	// Empty any artefacts that survived a previous run so the
	// scenario-C snapshot assertions don't see ghost objects, and
	// register a cleanup so we leave MinIO tidy.
	Expect(infra.EmptyBucket(ctx, s3Client, snapshotBucket)).To(Succeed())
	DeferCleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = infra.EmptyBucket(cleanCtx, s3Client, snapshotBucket)
	})

	// --- render configs ------------------------------------------------
	suite.EgressEnabled = os.Geteuid() == 0

	// The firecracker API socket lives at
	// <chroot>/firecracker/<sandbox-id>/root/firecracker.sock — and
	// Linux caps sun_path at 108 bytes. Our run-dir under the
	// project tree already eats ~75 chars, which blows the limit
	// once the daemon appends its bookkeeping suffix.
	//
	// /tmp would have headroom but is mounted nodev on Ubuntu, so
	// the jailer-created /dev/net/tun device node inside the
	// chroot can't be opened (firecracker exits with TapOpen
	// PermissionDenied). /var/tmp is on the root filesystem on
	// stock distros — short path AND device access. Logs and
	// configs stay in run-dir where the long path doesn't matter.
	chrootDir, err := os.MkdirTemp("/var/tmp", "yaghan-e2e-chroot-")
	Expect(err).NotTo(HaveOccurred(), "create chroot tempdir")
	DeferCleanup(func() { _ = os.RemoveAll(chrootDir) })

	apiCfg := filepath.Join(suite.ConfigDir, "api-server.yaml")
	daemonCfg := filepath.Join(suite.ConfigDir, "daemon.yaml")
	renderTemplate(filepath.Join("testdata", "api-server.yaml.tmpl"), apiCfg, nil)
	renderTemplate(filepath.Join("testdata", "daemon.yaml.tmpl"), daemonCfg, map[string]any{
		"AssetsDir":     suite.AssetsDir,
		"ChrootDir":     chrootDir,
		"SessionIDFile": filepath.Join(suite.RunDir, "session.id"),
		"EgressEnabled": suite.EgressEnabled,
	})

	// --- binaries on PATH ---------------------------------------------
	binDir := filepath.Join(suite.RepoRoot, "e2e", "bin")
	Expect(os.Setenv(yag.BinEnvVar, filepath.Join(binDir, "yag"))).To(Succeed())
	DeferCleanup(func() { _ = os.Unsetenv(yag.BinEnvVar) })

	// --- api-server ----------------------------------------------------
	apiProc, err := apiserverproc.Start(ctx, apiserverproc.Options{
		BinPath:    filepath.Join(binDir, "api-server"),
		ConfigPath: apiCfg,
		LogDir:     suite.RunDir,
	})
	Expect(err).NotTo(HaveOccurred(), "start api-server")
	DeferCleanup(func() { _ = apiProc.Stop(apiserverproc.GracefulStopTimeout) })

	Expect(infra.WaitForGRPCHealth(ctx, apiServerGRPCAddr)).To(Succeed(),
		"api-server gRPC health (log: %s)", apiProc.LogPath())

	// Open a gRPC client to the api-server for direct assertions in
	// BeforeSuite (waiting on registration) and for any spec that
	// wants the typed proto path rather than the yag CLI.
	suite.APIServerCC, err = grpc.NewClient(apiServerGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = suite.APIServerCC.Close() })
	suite.ClusterCli = controlplanev1alpha1.NewClusterServiceClient(suite.APIServerCC)
	suite.SandboxCli = controlplanev1alpha1.NewSandboxServiceClient(suite.APIServerCC)

	// --- daemon --------------------------------------------------------
	daemonProc, err := daemonproc.Start(ctx, daemonproc.Options{
		BinPath:    filepath.Join(binDir, "daemon"),
		ConfigPath: daemonCfg,
		LogDir:     suite.RunDir,
	})
	Expect(err).NotTo(HaveOccurred(), "start daemon")
	DeferCleanup(func() { _ = daemonProc.Stop(daemonproc.GracefulStopTimeout) })

	Expect(infra.WaitForGRPCHealth(ctx, daemonGRPCAddr)).To(Succeed(),
		"daemon gRPC health (log: %s)", daemonProc.LogPath())

	// --- daemon registers a healthy node ------------------------------
	nodeID, err := waitForRegisteredNode(ctx, suite.ClusterCli)
	Expect(err).NotTo(HaveOccurred(), "daemon never registered as healthy")
	suite.DaemonNodeID = nodeID
})

// preflight fails the suite early with a clear message when something
// the suite assumes is missing — sudo prompts, opaque "binary not
// found" errors deep in BeforeSuite, etc., are worse than an explicit
// "did you run hack/setup-dev.sh?" up front.
func preflight(assetsDir, binDir string) {
	for _, name := range []string{"firecracker", "jailer", "vmlinux", "rootfs.ext4"} {
		p := filepath.Join(assetsDir, name)
		_, err := os.Stat(p)
		Expect(err).NotTo(HaveOccurred(),
			"missing asset %s — run hack/setup-dev.sh from the repo root", p)
	}
	for _, name := range []string{"api-server", "daemon", "yag"} {
		p := filepath.Join(binDir, name)
		_, err := os.Stat(p)
		Expect(err).NotTo(HaveOccurred(),
			"missing binary %s — run `make binaries` from e2e/", p)
	}
	_, err := exec.LookPath("docker")
	Expect(err).NotTo(HaveOccurred(), "docker not on PATH")
}

func mustRepoRoot() string {
	// The test binary's CWD when `go test` runs is the package
	// directory (e2e/). Repo root is its parent.
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	return filepath.Dir(cwd)
}

func mustRunDir(repoRoot string) string {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(repoRoot, "e2e", "run-"+stamp)
	Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
	return dir
}

func renderTemplate(tmplPath, outPath string, data any) {
	raw, err := os.ReadFile(tmplPath)
	Expect(err).NotTo(HaveOccurred(), "read template %s", tmplPath)
	tmpl, err := template.New(filepath.Base(tmplPath)).Parse(string(raw))
	Expect(err).NotTo(HaveOccurred(), "parse template %s", tmplPath)
	out, err := os.Create(outPath)
	Expect(err).NotTo(HaveOccurred(), "create %s", outPath)
	defer func() { _ = out.Close() }()
	Expect(tmpl.Execute(out, data)).To(Succeed(), "render %s", tmplPath)
}

// waitForRegisteredNode blocks until ListNodes returns one node in
// PHASE_HEALTHY and returns that node's id. Polls every 500ms.
func waitForRegisteredNode(ctx context.Context, cli controlplanev1alpha1.ClusterServiceClient) (string, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		resp, err := cli.ListNodes(ctx, &controlplanev1alpha1.ListNodesRequest{
			StatusPhase: controlplanev1alpha1.NodeStatus_PHASE_HEALTHY,
			PageSize:    10,
		})
		switch {
		case err != nil:
			lastErr = err
		case len(resp.GetNodes()) >= 1:
			return resp.GetNodes()[0].GetMetadata().GetId(), nil
		default:
			lastErr = errors.New("no nodes in PHASE_HEALTHY yet")
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("wait for registered node: %w (last: %w)", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}
