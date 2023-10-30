package schemas

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// Service returns the common schema attributes between VCL/Compute services.
//
// NOTE: Some 'optional' attributes are also 'computed' so we can set a default.
// This is a requirement enforced on us by Terraform.
//
// NOTE: Some 'computed' attributes require a default to avoid test errors.
// If we don't set a default, the Create/Update methods have to explicitly set a
// value for the computed attributes. It's cleaner/easier to just set defaults.
func Service() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"activate": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Conditionally prevents the Service from being activated. The apply step will continue to create a new draft version but will not activate it if this is set to `false`. Default `true`",
			Optional:            true,
			Default:             booldefault.StaticBool(true),
		},
		"comment": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Description field for the service. Default `Managed by Terraform`",
			Optional:            true,
			Default:             stringdefault.StaticString("Managed by Terraform"),
		},
		"domains": schema.MapNestedAttribute{
			MarkdownDescription: "Each key within the map should be a unique identifier for the resources contained within. It is important to note that changing the key will delete and recreate the resource",
			Required:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						MarkdownDescription: "The domain that this Service will respond to",
						Required:            true,
					},
					"comment": schema.StringAttribute{
						MarkdownDescription: "An optional comment about the domain",
						Optional:            true,
					},
				},
			},
		},
		"force_destroy": schema.BoolAttribute{
			MarkdownDescription: "Services that are active cannot be destroyed. In order to destroy the service, set `force_destroy` to `true`. Default `false`",
			Optional:            true,
		},
		"force_refresh": schema.BoolAttribute{
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Used internally by the provider to temporarily indicate if all resources should call their associated API to update the local state. This is for scenarios where the service version has been reverted outside of Terraform (e.g. via the Fastly UI) and the provider needs to resync the state for a different active version (this is only if `activate` is `true`)",
		},
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Alphanumeric string identifying the service",
			PlanModifiers: []planmodifier.String{
				// UseStateForUnknown is useful for reducing (known after apply) plan
				// outputs for computed attributes which are known to not change over time.
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"imported": schema.BoolAttribute{
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Used internally by the provider to temporarily indicate if the service is being imported, and is reset to false once the import is finished",
		},
		"last_active": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "The last 'active' service version (typically in-sync with `version` but not if `activate` is `false`)",
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "The unique name for the service to create",
			Required:            true,
		},
		"reuse": schema.BoolAttribute{
			MarkdownDescription: "Services that are active cannot be destroyed. If set to `true` a service Terraform intends to destroy will instead be deactivated (allowing it to be reused by importing it into another Terraform project). If `false`, attempting to destroy an active service will cause an error. Default `false`",
			Optional:            true,
		},
		"version": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "The latest version that the provider will clone from (typically in-sync with `last_active` but not if `activate` is `false`)",
		},
	}
}
