package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ServiceActivation describes the resource data model.
type ServiceActivation struct {
	// Activate controls whether the service should be activated.
	Activate types.Bool `tfsdk:"activate"`
	// ID is for the associated service resource.
	ID types.String `tfsdk:"id"`
	// Version is the service version to activate.
	Version types.Int64 `tfsdk:"version"`
}
