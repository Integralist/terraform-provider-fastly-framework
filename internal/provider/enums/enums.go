package enums

// ServiceType is an enum for Fastly service types.
type ServiceType int

const (
	// VCL is a Delivery service type.
	VCL ServiceType = iota
	// Compute is a Compute@Edge service type.
	Compute
)

// NestedType is an enum for nested entities within a Fastly service type.
type NestedType int

const (
	Domain NestedType = iota
)
