package data

import (
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
)

// Resource is a wrapper to ensure nested entities implement
// interfaces.ResourceData (consumed by interfaces.Resource methods).
type Resource struct {
	// ServiceID is the ID for the Fastly service.
	ServiceID string
	// ServiceVersion is the current version for the Fastly service.
	ServiceVersion int32
	// Plan is the planned Terraform state changes.
	Plan any
	// State is the complete Terraform state data the nested model can reference.
	State any
	// Type is the service resource type (e.g. enums.VCL, enums.Compute)
	Type enums.ServiceType
}

func (r Resource) GetPlan() any {
	return r.Plan
}

func (r Resource) GetServiceID() string {
	return r.ServiceID
}

func (r Resource) GetServiceVersion() int32 {
	return r.ServiceVersion
}

func (r Resource) GetState() any {
	return r.State
}

func (r Resource) GetType() enums.ServiceType {
	return r.Type
}

func (r *Resource) SetPlan(plan any) {
	r.Plan = plan
}

func (r *Resource) SetState(state any) {
	r.State = state
}
