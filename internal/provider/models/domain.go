package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
)

// Domain is a nested set attribute for the domain(s) associated with a service.
type Domain struct {
	// Comment is an optional comment about the domain.
	Comment types.String `tfsdk:"comment"`
	// ID is a unique identifier used by the provider to determine changes within a nested set type.
	ID types.String `tfsdk:"id"`
	// Name is the domain that this service will respond to. It is important to note that changing this attribute will delete and recreate the resource.
	Name types.String `tfsdk:"name"`
}

// Service is a wrapper to ensure nested entities implement
// interfaces.NestedModel (consumed by interfaces.Resource methods).
type Service struct {
	Items []Domain
	// ServiceID is the ID for the Fastly service.
	ServiceID string
	// ServiceVersion is the current version for the Fastly service.
	ServiceVersion int32
	// State is the complete Terraform state data the nested model can reference.
	State any
	// Type is the nested model type (e.g. enums.Domain)
	Type enums.NestedType
}

func (d Service) GetNestedType() enums.NestedType {
	return d.Type
}

func (d Service) GetServiceID() string {
	return d.ServiceID
}

func (d Service) GetServiceVersion() int32 {
	return d.ServiceVersion
}
