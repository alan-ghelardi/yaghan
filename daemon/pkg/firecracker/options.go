package firecracker

// Options controls the behavior of the Firecracker [Provider]. Construct an
// Options value implicitly by passing [Option] values to [New].
type Options struct {
	// FirecrackerPath is the absolute path to the firecracker binary.
	// Must be a statically-linked musl build — the jailer chroots into an
	// empty filesystem so dynamic linker resolution is impossible.
	FirecrackerPath string

	// JailerPath is the absolute path to the jailer binary. Jailer is used
	// to sandbox each microVM in a per-VM chroot with a restricted uid/gid
	// and its own network namespace.
	JailerPath string

	// JailUID is the numeric user ID the jailer drops privileges to before
	// exec'ing firecracker. Typical deployments run Firecracker as an
	// unprivileged dedicated user (e.g. 1000).
	JailUID uint32

	// JailGID is the numeric group ID the jailer drops privileges to.
	JailGID uint32

	// ChrootBaseDir is the parent directory under which the jailer creates
	// per-VM chroots. Jailer lays out <ChrootBaseDir>/<exec-name>/<id>/root
	// for each VM; we put our own per-VM state (pid file, config) there
	// too. Defaults to "/srv/jailer" to match Firecracker's upstream docs.
	ChrootBaseDir string

	// AttachConsole controls whether firecracker's stdio (including the
	// guest serial console at ttyS0) is kept attached to the calling
	// process. When false (the default), jailer is invoked with
	// --daemonize, which redirects stdio to /dev/null and detaches the
	// process. Set to true for debugging — kernel boot messages and
	// guest PID 1 crashes then surface in the terminal. Only safe for
	// single-VM, foreground use.
	AttachConsole bool
}

// Option represents a single override applied to [Options].
type Option interface {
	Apply(opts *Options)
}

// OptionAdapter adapts a function to satisfy the Option interface.
type OptionAdapter func(opts *Options)

// Apply implements the Option interface.
func (f OptionAdapter) Apply(opts *Options) {
	f(opts)
}

// WithFirecrackerPath sets the path to the firecracker binary.
func WithFirecrackerPath(path string) Option {
	return OptionAdapter(func(opts *Options) {
		opts.FirecrackerPath = path
	})
}

// WithJailerPath sets the path to the jailer binary.
func WithJailerPath(path string) Option {
	return OptionAdapter(func(opts *Options) {
		opts.JailerPath = path
	})
}

// WithJailUID sets the numeric user ID the jailer drops privileges to.
func WithJailUID(uid uint32) Option {
	return OptionAdapter(func(opts *Options) {
		opts.JailUID = uid
	})
}

// WithJailGID sets the numeric group ID the jailer drops privileges to.
func WithJailGID(gid uint32) Option {
	return OptionAdapter(func(opts *Options) {
		opts.JailGID = gid
	})
}

// WithChrootBaseDir overrides the default chroot base directory
// ("/srv/jailer"). The jailer creates per-VM chroots beneath it.
func WithChrootBaseDir(dir string) Option {
	return OptionAdapter(func(opts *Options) {
		opts.ChrootBaseDir = dir
	})
}

// WithAttachConsole keeps firecracker attached to the calling process
// instead of daemonizing, so kernel boot messages and guest PID 1
// crashes stream to the terminal. Debug use only.
func WithAttachConsole(attach bool) Option {
	return OptionAdapter(func(opts *Options) {
		opts.AttachConsole = attach
	})
}
