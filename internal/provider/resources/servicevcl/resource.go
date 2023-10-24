package servicevcl

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/interfaces"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/resources/domain"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/schemas"
)

//go:embed docs/service_vcl.md
var resourceDescription string

// Ensure provider defined types fully satisfy framework interfaces.
//
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#Resource
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ResourceWithConfigValidators
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ResourceWithConfigure
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ResourceWithImportState
var (
	_ resource.Resource                     = &Resource{}
	_ resource.ResourceWithConfigValidators = &Resource{}
	_ resource.ResourceWithConfigure        = &Resource{}
	_ resource.ResourceWithImportState      = &Resource{}
)

// NewResource returns a new Terraform resource instance.
func NewResource() func() resource.Resource {
	return func() resource.Resource {
		return &Resource{
			nestedResources: []interfaces.Resource{
				domain.NewResource(),
			},
		}
	}
}

// Resource defines the resource implementation.
type Resource struct {
	// client is a preconfigured instance of the Fastly API client.
	client *fastly.APIClient
	// clientCtx contains the user's API token.
	clientCtx context.Context
	// nestedResources is a list of resources within the service resource.
	//
	// NOTE: Terraform doesn't have a concept of 'nested' resources.
	// We're using this terminology because it makes more sense for Fastly.
	// As our nested resources are actually just nested 'attributes'.
	// https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes#nested-attributes
	nestedResources []interfaces.Resource
}

// Metadata should return the full name of the resource.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_vcl"
}

// Schema should return the schema for this resource.
//
// NOTE: Some optional attributes are also 'computed' so we can set a default.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := schemas.Service()

	attrs["default_ttl"] = schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: "The default Time-to-live (TTL) for requests",
		Optional:            true,
		Default:             int64default.StaticInt64(3600),
	}
	attrs["default_host"] = schema.StringAttribute{
		MarkdownDescription: "The default hostname",
		Optional:            true,
	}
	attrs["stale_if_error"] = schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Enables serving a stale object if there is an error",
		Optional:            true,
		Default:             booldefault.StaticBool(false),
	}
	attrs["stale_if_error_ttl"] = schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: "The default time-to-live (TTL) for serving the stale object for the version",
		Optional:            true,
		Default:             int64default.StaticInt64(43200),
	}

	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: resourceDescription,

		// Attributes is the mapping of underlying attribute names to attribute definitions.
		Attributes: attrs,
	}
}

// Configure includes provider-level data or clients.
func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*fastly.APIClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *fastly.APIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
	r.clientCtx = fastly.NewAPIKeyContextFromEnv(helpers.APIKeyEnv)
}

// ImportState is called when the provider must import the state of a resource instance.
//
// The resource's ID is set into the state and its Read() method called.
// If we look at the Read() method in ./process_read.go we'll see it calls
// `ServiceAPI.GetServiceDetail()` passing in the ID the user specifies.
//
// e.g. `terraform import ADDRESS ID`
// https://developer.hashicorp.com/terraform/cli/commands/import#usage`
//
// The service resource then iterates over all nested resources populating the
// state for each nested resource.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// TODO: req.ID needs to be checked for format.
	// Typically just a Service ID but can also be <service id>@<service version>
	// If the @<service_version> format is provided, then we need to parse the
	// version and set it into the `version` attribute as well as `last_active`.

	// The ImportStatePassthroughID() call is a small helper function that simply
	// checks for an empty ID value passed (and errors accordingly) and if there
	// is no error it calls `resp.State.SetAttribute()` passing in the ADDRESS
	// (which we hardcode to the `id` attribute) and the user provided ID value.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	var state map[string]tftypes.Value
	err := resp.State.Raw.As(&state)
	if err == nil {
		tflog.Trace(ctx, "ImportState", map[string]any{"state": fmt.Sprintf("%#v", state)})
	}
}

// ConfigValidators returns a list of functions which will all be performed during validation.
// https://developer.hashicorp.com/terraform/plugin/framework/resources/validate-configuration#configvalidators-method
func (r Resource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("force_destroy"),
			path.MatchRoot("reuse"),
		),
	}
}
