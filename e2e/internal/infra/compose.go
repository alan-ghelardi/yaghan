// Package infra brings up the backing services (DynamoDB Local, Redis,
// MinIO) the api-server and daemon binaries need at runtime, and exposes
// readiness probes the suite uses to gate process startup.
//
// Everything here drives the existing dev compose stacks at
// api-server/dev/docker-compose.yml and daemon/dev/docker-compose.yml —
// the suite owns the project name (yaghan-e2e-*) so its containers don't
// collide with a developer running dev/start.sh in another terminal.
// (They do still collide on host ports — see e2e/README.md.)
package infra

import (
	"context"
	"fmt"
	"os/exec"
)

// Up runs `docker compose -p <project> -f <composeFile> up -d`. Blocks
// until docker reports the containers as created and started, but does
// not wait for the inner service (DynamoDB, Redis, MinIO) to accept
// requests — call the matching WaitFor* helper for that.
func Up(ctx context.Context, composeFile, project string) error {
	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-p", project, "-f", composeFile, "up", "-d")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up (%s): %w\n%s", composeFile, err, out)
	}
	return nil
}

// Down runs `docker compose -p <project> -f <composeFile> down
// --remove-orphans`. Safe to call even if Up was never run — docker
// compose treats an absent project as a no-op.
func Down(ctx context.Context, composeFile, project string) error {
	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-p", project, "-f", composeFile, "down", "--remove-orphans")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down (%s): %w\n%s", composeFile, err, out)
	}
	return nil
}
