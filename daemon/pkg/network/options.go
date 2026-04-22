package network

// Options controls the behavior of a [Driver]. Construct an Options value
// implicitly by passing [Option] values to [NewLinuxDriver].
type Options struct {
	// TAPOwner sets the uid allowed to attach to the TAP device
	// (vm-tap0). Required when firecracker runs under a non-root jailer
	// uid; otherwise TUNSETIFF returns EPERM. Zero means "leave unset",
	// which restricts attachment to root.
	TAPOwner uint32

	// TAPGroup sets the gid allowed to attach to the TAP device.
	// Zero means "leave unset".
	TAPGroup uint32
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

// WithTAPOwner sets the uid allowed to attach to the TAP device.
func WithTAPOwner(uid uint32) Option {
	return OptionAdapter(func(opts *Options) {
		opts.TAPOwner = uid
	})
}

// WithTAPGroup sets the gid allowed to attach to the TAP device.
func WithTAPGroup(gid uint32) Option {
	return OptionAdapter(func(opts *Options) {
		opts.TAPGroup = gid
	})
}
