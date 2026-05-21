package metrics

// VectorOpts is a general configuration for metric vectors.
type VectorOpts struct {
	Namespace string
	Subsystem string
	Name      string
	Help      string
	Labels    []string
}
