package data

import (
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
)

// Resource is a wrapper around top-level resource service data.
type Resource struct {
	// Plan is the planned Terraform state changes.
	Plan any
	// ServiceID is the ID for the Fastly service.
	ServiceID string
	// ServiceVersion is the current version for the Fastly service.
	ServiceVersion int32
	// State is the complete Terraform state data the nested model can reference.
	State any
	// Type is the service resource type (e.g. enums.VCL, enums.Compute)
	Type enums.ServiceType
}
