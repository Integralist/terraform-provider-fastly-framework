package provider

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"net/http"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
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
	// Domains is a block for the domain(s) associated with the service.
	Domains []ServiceDomain `tfsdk:"domains"`
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

// ServiceDomain is a block for the domain(s) associated with the service.
type ServiceDomain struct {
	// Comment is an optional comment about the domain.
	Comment types.String `tfsdk:"comment"`
	// ID is a unique identifier used by the provider to determine changes within a nested set type.
	ID types.String `tfsdk:"id"`
	// Name is the domain that this service will respond to. It is important to note that changing this attribute will delete and recreate the resource.
	Name types.String `tfsdk:"name"`
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
				// NOTE: This is marked computed so provider can set a default.
				Computed:            true,
				MarkdownDescription: "Description field for the service. Default `Managed by Terraform`",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					helpers.StringDefaultModifier{Default: "Managed by Terraform"},
				},
			},
			// FIXME: We should consider a MapNestedAttribute to avoid diff issues.
			// https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes#mapnestedattribute
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
	var plan *ServiceVCLResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clientReq := r.client.ServiceAPI.CreateService(r.clientCtx)
	clientReq.Comment(plan.Comment.ValueString())
	clientReq.Name(plan.Name.ValueString())
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
	plan.ID = types.StringValue(*id)

	versions, ok := clientResp.GetVersionsOk()
	if !ok {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", "No Service versions returned")
		return
	}
	version := versions[0].GetNumber()
	plan.Version = types.Int64Value(int64(version))

	if plan.Activate.ValueBool() {
		plan.LastActive = plan.Version
	}

	// TODO: Abstract domains (and other resources)
	// https://pkg.go.dev/github.com/mitchellh/mapstructure might help for update diffing.
	for i := range plan.Domains {
		domain := &plan.Domains[i]

		if domain.ID.IsUnknown() {
			// NOTE: We create a consistent hash of the domain name for the ID.
			// Originally I used github.com/google/uuid but thought it would be more
			// appropriate to use a hash of the domain name.
			digest := sha256.Sum256([]byte(domain.Name.ValueString()))
			domain.ID = types.StringValue(fmt.Sprintf("%x", digest))
		}

		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.CreateDomain(r.clientCtx, *id, int32(version))

		if !domain.Comment.IsNull() {
			clientReq.Comment(domain.Comment.ValueString())
		}

		if !domain.Name.IsNull() {
			clientReq.Name(domain.Name.ValueString())
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

	if plan.Activate.ValueBool() {
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
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Create", map[string]any{"state": plan})
}

// TODO: How to handle DeletedAt attribute.
// TODO: How to handle service type mismatch when importing.
// TODO: How to handle name/comment which are versionless and need `activate`.
func (r *ServiceVCLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *ServiceVCLResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clientReq := r.client.ServiceAPI.GetServiceDetail(r.clientCtx, state.ID.ValueString())
	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly ServiceAPI.GetServiceDetail error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to retrieve service details, got error: %s", err))
		return
	}

	state.Comment = types.StringValue(clientResp.GetComment())
	state.ID = types.StringValue(clientResp.GetID())
	state.Name = types.StringValue(clientResp.GetName())

	// NOTE: When importing a service there is no prior 'version' in the state.
	// So we presume the user wants to import the last active service version.
	// Which we retrieve from the GetServiceDetail call.
	var foundActive bool
	versions := clientResp.GetVersions()
	for _, version := range versions {
		if version.GetActive() {
			lastActive := int64(version.GetNumber())
			state.Version = types.Int64Value(lastActive)
			state.LastActive = types.Int64Value(lastActive)
			foundActive = true
			break
		}
	}

	// NOTE: In case user imports service with no active versions, use latest.
	if !foundActive {
		version := int64(versions[0].GetNumber())
		state.Version = types.Int64Value(version)
		state.LastActive = types.Int64Value(version)
	}

	// TODO: Abstract domains (and other resources) and rename back to clientReq.
	clientDomainReq := r.client.DomainAPI.ListDomains(r.clientCtx, clientResp.GetID(), int32(state.Version.ValueInt64()))

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

	var domains []ServiceDomain

	// TODO: Rename domainData to domain once moved to separate package.
	for _, domainData := range clientDomainResp {
		domainName := domainData.GetName()
		digest := sha256.Sum256([]byte(domainName))

		sd := ServiceDomain{
			ID: types.StringValue(fmt.Sprintf("%x", digest)),
		}

		// NOTE: We call the Ok variant of the API so we can check if value was set.
		// WARNING: The code doesn't work as you might expect because of the Fastly API.
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
		if v, ok := domainData.GetCommentOk(); !ok {
			sd.Comment = types.StringNull()
		} else {
			sd.Comment = types.StringValue(*v)

			// We need to check if the user config has set the comment.
			// If not, then we'll again set the value to null to avoid a plan diff.
			for _, stateDomain := range state.Domains {
				if stateDomain.Name.ValueString() == domainName {
					if stateDomain.Comment.IsNull() {
						sd.Comment = types.StringNull()
					}
				}
			}
		}

		if v, ok := domainData.GetNameOk(); !ok {
			sd.Name = types.StringNull()
		} else {
			sd.Name = types.StringValue(*v)
		}

		domains = append(domains, sd)
	}

	state.Domains = domains

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read", map[string]any{"state": state})
}

func (r *ServiceVCLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *ServiceVCLResourceModel
	var state *ServiceVCLResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read Terraform state data into the model so it can be compared against plan
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// NOTE: The plan data doesn't contain computed attributes.
	// So we need to read it from the current state.
	plan.Version = state.Version
	plan.LastActive = state.LastActive

	// NOTE: Name and Comment are versionless attributes.
	// Other nested blocks will need a new service version.
	var shouldClone bool

	// FIXME: We need something like SetDiff from the original provider.
	// We compare the plan set to the state set and determine what changed.
	// e.g. 'added', 'modified', 'deleted' and calls relevant API.
	// Needs a 'key' for each resource (sometimes 'name' but has to be unique).
	//
	// If plan 'key' exists in prior state, then it's modified.
	// Otherwise resource is new.
	// If state 'key' doesn't exist in plan, then it's deleted.
	var added, deleted, modified []ServiceDomain

	// If domain hashed is found in state, then it already exists and might be modified.
	// If domain hashed is not found in state, then it is either new or an existing domain that was renamed.
	// We then separately loop the state and see if it exists in the plan (if it doesn't, then it's deleted)

	// NOTE: We have to manually track each resource in a nested set block.
	// TODO: Abstract domain and other resources
	for i := range plan.Domains {
		// NOTE: We need a pointer to the resource struct so we can set an ID.
		planDomain := &plan.Domains[i]

		// ID is a computed value so we need to regenerate it from the domain name.
		if planDomain.ID.IsUnknown() {
			digest := sha256.Sum256([]byte(planDomain.Name.ValueString()))
			planDomain.ID = types.StringValue(fmt.Sprintf("%x", digest))
		}

		var foundDomain bool
		for _, stateDomain := range state.Domains {
			if planDomain.ID.ValueString() == stateDomain.ID.ValueString() {
				foundDomain = true
				// NOTE: It's not possible for the domain's Name field to not match.
				// This is because we first check the ID field matches, and that is
				// based on a hash of the domain name. Because of this we don't bother
				// checking if planDomain.Name and stateDomain.Name are not equal.
				if !planDomain.Comment.Equal(stateDomain.Comment) {
					shouldClone = true
					modified = append(modified, *planDomain)
				}
				break
			}
		}

		if !foundDomain {
			shouldClone = true
			added = append(added, *planDomain)
		}
	}

	for _, stateDomain := range state.Domains {
		var foundDomain bool
		for _, planDomain := range plan.Domains {
			if planDomain.ID.ValueString() == stateDomain.ID.ValueString() {
				foundDomain = true
				break
			}
		}

		if !foundDomain {
			shouldClone = true
			deleted = append(deleted, stateDomain)
		}
	}

	var serviceVersionToActivate int32
	if shouldClone {
		clientReq := r.client.VersionAPI.CloneServiceVersion(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()))
		clientResp, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly VersionAPI.CloneServiceVersion error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to clone service version, got error: %s", err))
			return
		}
		plan.Version = types.Int64Value(int64(clientResp.GetNumber()))

		// TODO: Figure out what this API does and why we call it?
		clientUpdateServiceVersionReq := r.client.VersionAPI.UpdateServiceVersion(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()))
		clientUpdateServiceVersionResp, httpResp, err := clientUpdateServiceVersionReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly VersionAPI.UpdateServiceVersion error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update service version, got error: %s", err))
			return
		}

		serviceVersionToActivate = clientUpdateServiceVersionResp.GetNumber()
	}

	for _, domain := range added {
		tflog.Debug(ctx, "domains", map[string]any{"added": added})

		// TODO: Abstract the following API call into a function as it's called multiple times.

		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.CreateDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()))

		if !domain.Comment.IsNull() {
			clientReq.Comment(domain.Comment.ValueString())
		}

		if !domain.Name.IsNull() {
			clientReq.Name(domain.Name.ValueString())
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

	for _, domain := range deleted {
		tflog.Debug(ctx, "domains", map[string]any{"deleted": deleted})

		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.DeleteDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()), domain.Name.ValueString())

		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly DomainAPI.DeleteDomain error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete domain, got error: %s", err))
			return
		}
		if httpResp.StatusCode != http.StatusOK {
			tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
			return
		}
	}

	for _, domain := range modified {
		tflog.Debug(ctx, "domains", map[string]any{"modified": modified})

		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.UpdateDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()), "")

		if !domain.Comment.IsNull() {
			clientReq.Comment(domain.Comment.ValueString())
		}

		// NOTE: We don't bother to check/update the domain's Name field.
		// This is because if the name of the domain has changed, then that means
		// the a new domain will be added and the original domain deleted. Thus,
		// we'll only have a domain as 'modified' if the Comment field was modified.

		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly DomainAPI.UpdateDomain error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update domain, got error: %s", err))
			return
		}
		if httpResp.StatusCode != http.StatusOK {
			tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
			return
		}
	}

	// NOTE: We have to manually track changes in each resource.
	for i := range plan.Domains {
		planDomain := &plan.Domains[i]

		for _, stateDomain := range state.Domains {
			if planDomain.ID.Equal(stateDomain.ID) {
				// If there are no changes in this resource's attributes, then skip update.
				if planDomain.Comment.Equal(stateDomain.Comment) && planDomain.Name.Equal(stateDomain.Name) {
					break
				}

				// TODO: Check if the version we have is correct.
				// e.g. should it be latest 'active' or just latest version?
				// It should depend on `activate` field but also whether the service pre-exists.
				// The service might exist if it was imported or a secondary config run.
				clientReq := r.client.DomainAPI.UpdateDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()), stateDomain.Name.ValueString())

				if !planDomain.Comment.IsNull() {
					clientReq.Comment(planDomain.Comment.ValueString())
				}

				if !planDomain.Name.IsNull() {
					clientReq.Name(planDomain.Name.ValueString())
				}

				_, httpResp, err := clientReq.Execute()
				if err != nil {
					tflog.Trace(ctx, "Fastly DomainAPI.UpdateDomain error", map[string]any{"http_resp": httpResp})
					resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update domain, got error: %s", err))
					return
				}
				if httpResp.StatusCode != http.StatusOK {
					tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
					resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
					return
				}

				break
			}
		}
	}

	// NOTE: UpdateService doesn't take a version because its attributes are versionless.
	// When cloning (see above) we need to call UpdateServiceVersion.
	clientReq := r.client.ServiceAPI.UpdateService(r.clientCtx, plan.ID.ValueString())
	if !plan.Comment.Equal(state.Comment) {
		clientReq.Comment(plan.Comment.ValueString())
	}
	if !plan.Name.Equal(state.Name) {
		clientReq.Name(plan.Name.ValueString())
	}

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly ServiceAPI.UpdateService error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update service, got error: %s", err))
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if shouldClone {
		clientReq := r.client.VersionAPI.ActivateServiceVersion(r.clientCtx, plan.ID.ValueString(), serviceVersionToActivate)
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly VersionAPI.ActivateServiceVersion error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate service version %d, got error: %s", 1, err))
			return
		}
	}

	tflog.Debug(ctx, "Update", map[string]any{"state": plan})
}

func (r *ServiceVCLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *ServiceVCLResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if (state.Force.ValueBool() || state.Reuse.ValueBool()) && state.Activate.ValueBool() {
		clientReq := r.client.ServiceAPI.GetServiceDetail(r.clientCtx, state.ID.ValueString())
		clientResp, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly ServiceAPI.GetServiceDetail error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to retrieve service details, got error: %s", err))
			return
		}

		version := *clientResp.GetActiveVersion().Number

		if version != 0 {
			clientReq := r.client.VersionAPI.DeactivateServiceVersion(r.clientCtx, state.ID.ValueString(), version)
			_, httpResp, err := clientReq.Execute()
			if err != nil {
				tflog.Trace(ctx, "Fastly VersionAPI.DeactivateServiceVersion error", map[string]any{"http_resp": httpResp})
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to deactivate service version %d, got error: %s", version, err))
				return
			}
		}
	}

	if !state.Reuse.ValueBool() {
		clientReq := r.client.ServiceAPI.DeleteService(r.clientCtx, state.ID.ValueString())
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly ServiceAPI.DeleteService error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete service, got error: %s", err))
			return
		}
	}

	tflog.Debug(ctx, "Delete", map[string]any{"state": state})
}

func (r *ServiceVCLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// TODO: req.ID needs to be checked for format.
	// Typically just a Service ID but can also be <service id>@<service version>
	// If the @<service_version> format is provided, then we need to parse the
	// version and set it into the `version` attribute as well as `last_active`.

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	var state map[string]tftypes.Value
	err := resp.State.Raw.As(&state)
	if err == nil {
		tflog.Debug(ctx, "ImportState", map[string]any{"state": state})
	}
}
