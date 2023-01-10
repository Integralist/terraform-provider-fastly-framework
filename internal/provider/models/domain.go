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

// Domains is a wrapper to ensure domain entities implement
// interfaces.NestedModel (consumed by interfaces.Resource methods).
type Domains struct {
	Items []Domain
}

func (d Domains) GetType() enums.NestedType {
	return enums.Domain
}
