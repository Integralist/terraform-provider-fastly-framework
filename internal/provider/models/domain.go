package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Domain is a nested map attribute for the domain(s) associated with a service.
type Domain struct {
	// Comment is an optional comment about the domain.
	Comment types.String `tfsdk:"comment"`
	// Name is a required field representing the domain name.
	Name types.String `tfsdk:"name"`
	// NamePast is internally used for tracking changes.
	NamePast types.String `tfsdk:"-"`
}
