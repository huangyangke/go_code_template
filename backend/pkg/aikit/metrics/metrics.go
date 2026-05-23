package metrics

var registry []func()

// Register adds an initializer that will be called when Enable is invoked.
// Call this from init() in any package that owns metrics.
func Register(fn func()) {
	registry = append(registry, fn)
}

// Enable runs all registered initializers. Call once at startup when
// Prometheus scraping is enabled.
func Enable() {
	for _, fn := range registry {
		fn()
	}
}

// VectorOpts is a general configuration for metric vectors.
type VectorOpts struct {
	Namespace string
	Subsystem string
	Name      string
	Help      string
	Labels    []string
}
