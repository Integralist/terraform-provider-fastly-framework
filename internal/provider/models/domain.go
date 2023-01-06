package models

import "github.com/hashicorp/terraform-plugin-framework/types"

// ServiceDomain is a nested set attribute for the domain(s) associated with a service.
type ServiceDomain struct {
	// Comment is an optional comment about the domain.
	Comment types.String `tfsdk:"comment"`
	// ID is a unique identifier used by the provider to determine changes within a nested set type.
	ID types.String `tfsdk:"id"`
	// Name is the domain that this service will respond to. It is important to note that changing this attribute will delete and recreate the resource.
	Name types.String `tfsdk:"name"`
}
