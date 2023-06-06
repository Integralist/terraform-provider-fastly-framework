package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Thing describes the resource data model.
type Thing struct {
	// ID is for the associated service resource.
	ID types.String `tfsdk:"id"`
	// Version is the service version to activate.
	Version types.Int64 `tfsdk:"version"`
}
