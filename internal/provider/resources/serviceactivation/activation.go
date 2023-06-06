package serviceactivation

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
)

//go:embed docs/service_activation.md
var resourceDescription string

// Ensure provider defined types fully satisfy framework interfaces
//
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#Resource
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ResourceWithConfigure
var (
	_ resource.Resource              = &Resource{}
	_ resource.ResourceWithConfigure = &Resource{}
)

// NewResource returns a new Terraform resource instance.
func NewResource() func() resource.Resource {
	return func() resource.Resource {
		return &Resource{}
	}
}

// Resource defines the resource implementation.
type Resource struct {
	// client is a preconfigured instance of the Fastly API client.
	client *fastly.APIClient
	// clientCtx contains the user's API token.
	clientCtx context.Context
}

// Metadata should return the full name of the resource.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_activation"
}

// Schema should return the schema for this resource.
//
// NOTE: Some optional attributes are also 'computed' so we can set a default.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"activate": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether to activate the service (true) or to leave it inactive (false).",
			Optional:            true,
			PlanModifiers: []planmodifier.Bool{
				helpers.BoolDefaultModifier{Default: true},
			},
		},
		"id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Alphanumeric string identifying the associated service resource",
			PlanModifiers: []planmodifier.String{
				// UseStateForUnknown is useful for reducing (known after apply) plan
				// outputs for computed attributes which are known to not change over time.
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"version": schema.Int64Attribute{
			Required:            true,
			MarkdownDescription: "The associated service version to activate",
		},
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
	r.clientCtx = fastly.NewAPIKeyContextFromEnv("FASTLY_API_TOKEN")
}
