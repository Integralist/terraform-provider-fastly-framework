package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Domain is a nested map attribute for the domain(s) associated with a service.
//
// NOTE: The domain 'name' is the map key (see ../schemas/service.go)
type Domain struct {
	// Comment is an optional comment about the domain.
	Comment types.String `tfsdk:"comment"`
}
