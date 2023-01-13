package data

// Resource is a wrapper around top-level resource service data.
type Resource struct {
	// ServiceID is the ID for the Fastly service.
	ServiceID string
	// ServiceVersion is the current version for the Fastly service.
	ServiceVersion int32
}
