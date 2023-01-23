package schemas

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
)

// Service returns the common schema attributes between VCL/Compute services.
//
// NOTE: Some optional attributes are also 'computed' so we can set a default.
func Service() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"activate": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Conditionally prevents the Service from being activated. The apply step will continue to create a new draft version but will not activate it if this is set to `false`. Default `true`",
			Optional:            true,
			PlanModifiers: []planmodifier.Bool{
				helpers.BoolDefaultModifier{Default: true},
			},
		},
		"comment": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Description field for the service. Default `Managed by Terraform`",
			Optional:            true,
			PlanModifiers: []planmodifier.String{
				helpers.StringDefaultModifier{Default: "Managed by Terraform"},
			},
		},
		"domains": schema.SetNestedAttribute{
			Required: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"comment": schema.StringAttribute{
						MarkdownDescription: "An optional comment about the domain",
						Optional:            true,
					},
					"id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Unique identifier used by the provider to determine changes within a nested set type",
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "The domain that this service will respond to. It is important to note that changing this attribute will delete and recreate the resource",
						Required:            true,
					},
				},
			},
		},
		"force": schema.BoolAttribute{
			MarkdownDescription: "Services that are active cannot be destroyed. In order to destroy the service, set `force_destroy` to `true`. Default `false`",
			Optional:            true,
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
