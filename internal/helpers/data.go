package helpers

// Service is a wrapper around top-level resource service data.
type Service struct {
	// ID is the ID for the Fastly service.
	ID string
	// Version is the current version for the Fastly service.
	Version int32

	// TODO: Consider updating the fastly-go API client to use int64.
	//
	// This is because Terraform doesn't support int32 (https://github.com/hashicorp/terraform-plugin-framework/issues/801).
	// The change would be more to help the readability of the Terraform provider code.
	// As we wouldn't need the visual noise of constantly converting between 32 and 64 types.
	//
	// Although, strictly speaking, downsizing an int64 to int32 could cause data
	// loss, the reality is an int32 largest value is 2,147,483,647 and it's
	// doubtful that a user service will contain that many versions (in practice).
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
