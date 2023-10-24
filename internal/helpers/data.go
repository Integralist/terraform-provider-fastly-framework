package helpers

// Service is a wrapper around top-level resource service data.
type Service struct {
	// ID is the ID for the Fastly service.
	ID string
	// Version is the current version for the Fastly service.
	Version int32
}

// ServiceType is a base for the different service variants.
type ServiceType int64

// Stringer implements the Stringer interface.
// This enables conversion from int64 to appropriate string value.
func (p ServiceType) String() string {
	switch p {
	case ServiceTypeVCL:
		return "vcl"
	case ServiceTypeWasm:
		return "wasm"
	case ServiceTypeUndefined:
		return "undefined"
	}
	return "unknown"
}

const (
	ServiceTypeUndefined ServiceType = iota
	ServiceTypeVCL
	ServiceTypeWasm
)
