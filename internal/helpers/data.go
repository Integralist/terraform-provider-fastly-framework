package helpers

// Service is a wrapper around top-level resource service data.
type Service struct {
	// ID is the ID for the Fastly service.
	ID string
	// Version is the current version for the Fastly service.
	Version int32
}
