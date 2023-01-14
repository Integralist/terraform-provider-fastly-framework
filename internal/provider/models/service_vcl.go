package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ServiceVCL describes the resource data model.
type ServiceVCL struct {
	// Activate controls whether the service should be activated.
	Activate types.Bool `tfsdk:"activate"`
	// Comment is a description field for the service.
	Comment types.String `tfsdk:"comment"`
	// DefaultHost is the default host name for the version.
	DefaultHost types.String `tfsdk:"default_host"`
	// DefaultTTL is the default time-to-live (TTL) for the version.
	DefaultTTL types.Int64 `tfsdk:"default_ttl"`
	// Domains is a nested set attribute for the domain(s) associated with the service.
	Domains []Domain `tfsdk:"domains"`
	// Force ensures a service will be fully deleted upon `terraform destroy`.
	Force types.Bool `tfsdk:"force"`
	// ID is a unique ID for the service.
	ID types.String `tfsdk:"id"`
	// LastActive is the last known active service version.
	LastActive types.Int64 `tfsdk:"last_active"`
	// Name is the service name.
	Name types.String `tfsdk:"name"`
	// Reuse will not delete the service upon `terraform destroy`.
	Reuse types.Bool `tfsdk:"reuse"`
	// StaleIfError enables serving a stale object if there is an error.
	StaleIfError types.Bool `tfsdk:"stale_if_error"`
	// StaleIfErrorTTL is the default time-to-live (TTL) for serving the stale object for the version.
	StaleIfErrorTTL types.Int64 `tfsdk:"stale_if_error_ttl"`
	// Version is the latest service version the provider will clone from.
	Version types.Int64 `tfsdk:"version"`
}
