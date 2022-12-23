package provider

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"strconv"

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
	// client is a preconfigured instance of the Fastly API client.
	client *fastly.APIClient
	// clientCtx contains the user's API token.
	clientCtx context.Context
}

// ServiceVCLResourceModel describes the resource data model.
type ServiceVCLResourceModel struct {
	// Activate controls whether the service should be activated.
	Activate types.Bool `tfsdk:"activate"`
	// Comment is a description field for the service.
	Comment types.String `tfsdk:"comment"`
	// Domain is a block for the domain(s) associated with the service.
	Domain types.Set `tfsdk:"domain"`
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
	// Version is the latest service version the provider will clone from.
	Version types.Int64 `tfsdk:"version"`
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
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", "No Service ID was returned")
		return
	}
	data.ID = types.StringValue(*id)

	versions, ok := clientResp.GetVersionsOk()
	if !ok {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", "No Service versions returned")
		return
	}
	version := versions[0].GetNumber()
	data.Version = types.Int64Value(int64(version))

	// NOTE: Update function doesn't have access to state, only plan data.
	// This means we need to persist the service version to private state.
	// But the interface means we must marshal data to []byte.
	privateVersion := []byte(strconv.Itoa(int(version)))
	resp.Private.SetKey(ctx, "version", privateVersion)

	if data.Activate.ValueBool() {
		data.LastActive = data.Version

		// NOTE: Update function doesn't have access to state, only plan data.
		// This means we need to persist the service version to private state.
		// But the interface means we must marshal data to []byte.
		resp.Private.SetKey(ctx, "last_active", privateVersion)
	}

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

		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.CreateDomain(r.clientCtx, *id, int32(version))

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
		// FIXME: Need to check for changes + service already active.
		clientReq := r.client.VersionAPI.ActivateServiceVersion(r.clientCtx, *id, int32(version))
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

	// NOTE: When importing a service there is no prior 'version' in the state.
	// So we presume the user wants to import the last active service version.
	// Which we retrieve from the GetServiceDetail call.
	var foundActive bool
	versions := clientResp.GetVersions()
	for _, version := range versions {
		if version.GetActive() {
			lastActive := int64(version.GetNumber())
			data.Version = types.Int64Value(lastActive)
			data.LastActive = types.Int64Value(lastActive)
			foundActive = true
			break
		}
	}

	// NOTE: In case user imports service with no active versions, use latest.
	if !foundActive {
		version := int64(versions[0].GetNumber())
		data.Version = types.Int64Value(version)
		data.LastActive = types.Int64Value(version)
	}

	// TODO: Abstract domains (and other resources) and rename back to clientReq.
	clientDomainReq := r.client.DomainAPI.ListDomains(r.clientCtx, clientResp.GetID(), int32(data.Version.ValueInt64()))

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

	version, diag := resp.Private.GetKey(ctx, "version")

	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	i, err := strconv.Atoi(string(version))
	if err != nil {
		tflog.Trace(ctx, "Private data conversion error", map[string]any{"err": err})
		resp.Diagnostics.AddError("Private data conversion error", fmt.Sprintf("Unable to convert data to int: %s", err))
		return
	}

	// NOTE: Update function doesn't have access to state, only plan data.
	// This means we needed to persist the service version (via Create) to private state.
	// But the interface means we must marshal data from []byte.
	data.Version = types.Int64Value(int64(i))

	lastActive, diag := resp.Private.GetKey(ctx, "last_active")

	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	active, err := strconv.Atoi(string(lastActive))
	if err != nil {
		tflog.Trace(ctx, "Private data conversion error", map[string]any{"err": err})
		resp.Diagnostics.AddError("Private data conversion error", fmt.Sprintf("Unable to convert data to int: %s", err))
		return
	}

	// NOTE: Update function doesn't have access to state, only plan data.
	// This means we needed to persist the last active service version (via Create) to private state.
	// But the interface means we must marshal data from []byte.
	data.LastActive = types.Int64Value(int64(active))

	// TODO: Implement Update logic
	//
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
	// TODO: req.ID needs to be checked for format.
	// Typically just a Service ID but can also be <service id>@<service version>
	// Refer to the original provider's implementation for details.

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
