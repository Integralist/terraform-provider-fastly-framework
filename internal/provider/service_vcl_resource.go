package provider

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
)

//go:embed docs/service_vcl.md
var resourceDescription string

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ resource.Resource                     = &ServiceVCLResource{}
	_ resource.ResourceWithImportState      = &ServiceVCLResource{}
	_ resource.ResourceWithConfigValidators = &ServiceVCLResource{}
)

func NewServiceVCLResource() resource.Resource {
	return &ServiceVCLResource{}
}

// ServiceVCLResource defines the resource implementation.
type ServiceVCLResource struct {
	client    *fastly.APIClient
	clientCtx context.Context
}

// ServiceVCLResourceModel describes the resource data model.
type ServiceVCLResourceModel struct {
	Activate types.Bool   `tfsdk:"activate"`
	Comment  types.String `tfsdk:"comment"`
	Domain   types.Set    `tfsdk:"domain"`
	Force    types.Bool   `tfsdk:"force"`
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Reuse    types.Bool   `tfsdk:"reuse"`
}

func (r *ServiceVCLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_vcl"
}

func (r *ServiceVCLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: resourceDescription,

		Attributes: map[string]schema.Attribute{
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
			"force": schema.BoolAttribute{
				MarkdownDescription: "Services that are active cannot be destroyed. In order to destroy the service, set `force_destroy` to `true`. Default `false`",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Example identifier",
				PlanModifiers: []planmodifier.String{
					// UseStateForUnknown is useful for reducing (known after apply) plan
					// outputs for computed attributes which are known to not change over time.
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The unique name for the service to create",
				Required:            true,
			},
			"reuse": schema.BoolAttribute{
				MarkdownDescription: "Services that are active cannot be destroyed. If set to `true` a service Terraform intends to destroy will instead be deactivated (allowing it to be reused by importing it into another Terraform project). If `false`, attempting to destroy an active service will cause an error. Default `false`",
				Optional:            true,
			},
		},

		// IMPORTANT: We might want to consider switching to nested attributes.
		// As nested blocks require much more complex code.
		// https://discuss.hashicorp.com/t/how-to-set-types-set-from-read-method/48146
		Blocks: map[string]schema.Block{
			"domain": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"comment": schema.StringAttribute{
							MarkdownDescription: "An optional comment about the domain",
							Optional:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The domain that this service will respond to. It is important to note that changing this attribute will delete and recreate the resource",
							Required:            true,
						},
					},
				},
			},
		},
	}
}

func (r ServiceVCLResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("force"),
			path.MatchRoot("reuse"),
		),
	}
}

func (r *ServiceVCLResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceVCLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *ServiceVCLResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clientReq := r.client.ServiceAPI.CreateService(r.clientCtx)
	clientReq.Comment(data.Comment.ValueString())
	clientReq.Name(data.Name.ValueString())
	clientReq.ResourceType("vcl")

	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly ServiceAPI.CreateService error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create service, got error: %s", err))
		return
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return
	}

	id, ok := clientResp.GetIDOk()
	if !ok {
		tflog.Trace(ctx, "Fastly ServiceAPI.CreateService error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", "No Service ID was returned")
		return
	}
	data.ID = types.StringValue(*id)

	// TODO: Abstract domains (and other resources)
	for _, domain := range data.Domain.Elements() {
		// TODO: abstract the conversion code as it's repeated in the Read function.
		v, err := domain.ToTerraformValue(ctx)
		if err != nil {
			tflog.Trace(ctx, "ToTerraformValue error", map[string]any{"err": err})
			resp.Diagnostics.AddError("ToTerraformValue error", fmt.Sprintf("Unable to convert type to Terraform value: %s", err))
			return
		}

		var dst map[string]tftypes.Value

		err = v.As(&dst)
		if err != nil {
			tflog.Trace(ctx, "As error", map[string]any{"err": err})
			resp.Diagnostics.AddError("As error", fmt.Sprintf("Unable to convert type to Go value: %s", err))
			return
		}

		// FIXME: The version number 1 needs to be the latest version or active.
		clientReq := r.client.DomainAPI.CreateDomain(r.clientCtx, *id, 1)

		if v, ok := dst["comment"]; ok && !v.IsNull() {
			var dst string
			err := v.As(&dst)
			if err != nil {
				tflog.Trace(ctx, "As error", map[string]any{"err": err})
				resp.Diagnostics.AddError("As error", fmt.Sprintf("Unable to convert type to Go value: %s", err))
				return
			}
			clientReq.Comment(dst)
		}

		if v, ok := dst["name"]; ok && !v.IsNull() {
			var dst string
			err := v.As(&dst)
			if err != nil {
				tflog.Trace(ctx, "As error", map[string]any{"err": err})
				resp.Diagnostics.AddError("As error", fmt.Sprintf("Unable to convert type to Go value: %s", err))
				return
			}
			clientReq.Name(dst)
		}

		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly DomainAPI.CreateDomain error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create domain, got error: %s", err))
			return
		}
		if httpResp.StatusCode != http.StatusOK {
			tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
			return
		}
	}

	if data.Activate.ValueBool() {
		clientReq := r.client.VersionAPI.ActivateServiceVersion(r.clientCtx, *id, 1)
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly VersionAPI.ActivateServiceVersion error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate service version %d, got error: %s", 1, err))
			return
		}
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// TODO: How to handle DeletedAt attribute.
// TODO: How to handle service type mismatch when importing.
// TODO: How to handle name/comment which are versionless and need `activate`.
func (r *ServiceVCLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *ServiceVCLResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clientReq := r.client.ServiceAPI.GetServiceDetail(r.clientCtx, data.ID.ValueString())
	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly ServiceAPI.GetServiceDetail error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to retrieve service details, got error: %s", err))
		return
	}

	data.Comment = types.StringValue(clientResp.GetComment())
	data.ID = types.StringValue(clientResp.GetID())
	data.Name = types.StringValue(clientResp.GetName())

	// TODO: Abstract domains (and other resources) and rename back to clientReq.
	clientDomainReq := r.client.DomainAPI.ListDomains(r.clientCtx, clientResp.GetID(), 1)

	// TODO: After abstraction rename back to clientResp.
	clientDomainResp, httpResp, err := clientDomainReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.ListDomains error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list domains, got error: %s", err))
		return
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return
	}

	attrTypes := map[string]attr.Type{
		"comment": types.StringType,
		"name":    types.StringType,
	}

	elementType := types.ObjectType{
		AttrTypes: attrTypes,
	}

	elements := []attr.Value{}

	for _, domain := range clientDomainResp {
		m := make(map[string]attr.Value)
		// NOTE: We call the Ok variant of the API so we can check if value was set.
		// WARNING: The code doesn't work as expected because of the Fastly API.
		//
		// The Fastly API always returns an empty string (and not null). This means
		// we get a conflict with the state file, as it stores the value as <null>.
		//
		// To avoid a "plan was not empty" test error, because Terraform sees the
		// value was <null> in the state but now is an empty string, we need to
		// check the state to see if the comment was originally <null> and to force
		// setting it back to null rather than using the empty string from the API.
		//
		// Ideally the Fastly API would return null or omit the field so the API
		// client could handle whether the value returned was null. So I've left the
		// conditional logic here but have added to the else statement additional
		// logic for working around the issue with the Fastly API response.
		if v, ok := domain.GetCommentOk(); !ok {
			m["comment"] = types.StringNull()
		} else {
			m["comment"] = types.StringValue(*v)

			// NOTE: The following logic works around lack of state when importing.
			if data.Domain.IsNull() {
				m["comment"] = types.StringNull()
			}

			// NOTE: The following logic works around the Fastly API (see above).
			for _, domain := range data.Domain.Elements() {
				// TODO: abstract the conversion code as it's the same in Create.
				v, err := domain.ToTerraformValue(ctx)
				if err != nil {
					tflog.Trace(ctx, "ToTerraformValue error", map[string]any{"err": err})
					resp.Diagnostics.AddError("ToTerraformValue error", fmt.Sprintf("Unable to convert type to Terraform value: %s", err))
					return
				}

				var dst map[string]tftypes.Value

				err = v.As(&dst)
				if err != nil {
					tflog.Trace(ctx, "As error", map[string]any{"err": err})
					resp.Diagnostics.AddError("As error", fmt.Sprintf("Unable to convert type to Go value: %s", err))
					return
				}

				if v, ok := dst["comment"]; ok && v.IsNull() {
					m["comment"] = types.StringNull()
				}
			}
		}

		if v, ok := domain.GetNameOk(); !ok {
			m["name"] = types.StringNull()
		} else {
			m["name"] = types.StringValue(*v)
		}

		elements = append(elements, types.ObjectValueMust(attrTypes, m))
	}

	set, diag := types.SetValue(elementType, elements)

	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Domain = set

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ServiceVCLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *ServiceVCLResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	fmt.Printf("update plan: %+v\n", data)

	// clientReq := r.client.ServiceAPI.UpdateService(r.clientCtx, data.ID.ValueString())
	//
	// clientResp, httpResp, err := clientReq.Execute()
	// if err != nil {
	// 	tflog.Trace(ctx, "Fastly ServiceAPI.UpdateService error", map[string]any{"http_resp": httpResp})
	// 	resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update service, got error: %s", err))
	// 	return
	// }

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ServiceVCLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *ServiceVCLResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if (data.Force.ValueBool() || data.Reuse.ValueBool()) && data.Activate.ValueBool() {
		clientReq := r.client.ServiceAPI.GetServiceDetail(r.clientCtx, data.ID.ValueString())
		clientResp, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly ServiceAPI.GetServiceDetail error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to retrieve service details, got error: %s", err))
			return
		}

		version := *clientResp.GetActiveVersion().Number

		if version != 0 {
			clientReq := r.client.VersionAPI.DeactivateServiceVersion(r.clientCtx, data.ID.ValueString(), version)
			_, httpResp, err := clientReq.Execute()
			if err != nil {
				tflog.Trace(ctx, "Fastly VersionAPI.DeactivateServiceVersion error", map[string]any{"http_resp": httpResp})
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to deactivate service version %d, got error: %s", version, err))
				return
			}
		}
	}

	if !data.Reuse.ValueBool() {
		clientReq := r.client.ServiceAPI.DeleteService(r.clientCtx, data.ID.ValueString())
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly ServiceAPI.DeleteService error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete service, got error: %s", err))
			return
		}
	}
}

func (r *ServiceVCLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
