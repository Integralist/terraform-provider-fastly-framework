package resources

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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/schemas"
)

//go:embed docs/service_vcl.md
var resourceDescription string

// Ensure provider defined types fully satisfy framework interfaces
//
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#Resource
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ResourceWithConfigValidators
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ResourceWithConfigure
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ResourceWithImportState
var (
	_ resource.Resource                     = &ServiceVCLResource{}
	_ resource.ResourceWithConfigValidators = &ServiceVCLResource{}
	_ resource.ResourceWithConfigure        = &ServiceVCLResource{}
	_ resource.ResourceWithImportState      = &ServiceVCLResource{}
)

// NewServiceVCLResource returns a new Terraform resource instance.
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

// Metadata should return the full name of the resource.
func (r *ServiceVCLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_vcl"
}

// Schema should return the schema for this resource.
func (r *ServiceVCLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := schemas.Service()

	// TODO: Implement Service settings logic.
	// https://developer.fastly.com/reference/api/vcl-services/settings/
	attrs["default_ttl"] = schema.Int64Attribute{
		MarkdownDescription: "The default Time-to-live (TTL) for requests",
		Optional:            true,
		PlanModifiers: []planmodifier.Int64{
			helpers.Int64DefaultModifier{Default: 3600},
		},
	}
	attrs["default_host"] = schema.StringAttribute{
		MarkdownDescription: "The default hostname",
		Optional:            true,
	}
	attrs["stale_if_error"] = schema.BoolAttribute{
		MarkdownDescription: "Enables serving a stale object if there is an error",
		Optional:            true,
		PlanModifiers: []planmodifier.Bool{
			helpers.BoolDefaultModifier{Default: false},
		},
	}
	attrs["stale_if_error_ttl"] = schema.Int64Attribute{
		MarkdownDescription: "The default time-to-live (TTL) for serving the stale object for the version",
		Optional:            true,
		PlanModifiers: []planmodifier.Int64{
			helpers.Int64DefaultModifier{Default: 43200},
		},
	}

	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: resourceDescription,

		// Attributes is the mapping of underlying attribute names to attribute definitions.
		Attributes: attrs,
	}
}

// Create is called when the provider must create a new resource.
// Config and planned state values should be read from the CreateRequest.
// New state values set on the CreateResponse.
func (r *ServiceVCLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *models.ServiceVCLResourceModel

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

	// TODO: Ensure API errors are managed accordingly.
	// https://github.com/fastly/terraform-provider-fastly/issues/631
	// This might not be a problem with the new framework.

	if err := DomainCreate(ctx, r, plan.Domains, *id, version, resp); err != nil {
		return
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

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Trace(ctx, "Create", map[string]any{"state": fmt.Sprintf("%+v", plan)})
}

// Read is called when the provider must read resource values in order to update state.
// Planned state values should be read from the ReadRequest.
// New state values set on the ReadResponse.
//
// TODO: How to handle DeletedAt attribute.
// TODO: How to handle service type mismatch when importing.
// TODO: How to handle name/comment which are versionless and need `activate`.
func (r *ServiceVCLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *models.ServiceVCLResourceModel

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

	if err := DomainRead(ctx, r, clientResp, state, resp); err != nil {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Trace(ctx, "Read", map[string]any{"state": fmt.Sprintf("%+v", state)})
}

// Update is called to update the state of the resource.
// Config, planned state, and prior state values should be read from the UpdateRequest.
// New state values set on the UpdateResponse.
func (r *ServiceVCLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *models.ServiceVCLResourceModel
	var state *models.ServiceVCLResourceModel

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

	// NOTE: Name and Comment are 'versionless' attributes.
	// Other nested attributes will need a new service version.
	var shouldClone bool

	// FIXME: We need an abstractions like SetDiff from the original provider.
	// We compare the plan set to the state set and determine what changed.
	// e.g. 'added', 'modified', 'deleted' and calls relevant API.
	// Needs a 'key' for each resource (sometimes 'name' but has to be unique).
	// Unless we switch to MapNestedAttribute which provides a key by design.
	//
	// If plan 'key' exists in prior state, then it's modified.
	// Otherwise resource is new.
	// If state 'key' doesn't exist in plan, then it's deleted.
	var added, deleted, modified []models.Domain

	// If domain hashed is found in state, then it already exists and might be modified.
	// If domain hashed is not found in state, then it is either new or an existing domain that was renamed.
	// We then separately loop the state and see if it exists in the plan (if it doesn't, then it's deleted)

	// NOTE: We have to manually track each resource in a nested set attribute.
	// For domains this means computing an ID for each domain, then calculating
	// whether a domain has been added, deleted or modified. If any of those
	// conditions are met, then we must clone the current service version.
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

	// IMPORTANT: We need to delete, then add, then update.
	// Some Fastly resources (like snippets) must have unique names.
	// If a user tries to switch from dynamicsnippet to snippet, and we don't
	// delete the resource first before creating the new one, then the Fastly API
	// will return an error and indicate that we have a conflict.
	//
	// FIXME: In the current Fastly provider there is a race condition bug.
	// https://github.com/fastly/terraform-provider-fastly/issues/628#issuecomment-1372477539
	// Which is based on the fact that snippets are two separate types.
	// We should make them a single type (as the API is one endpoint).
	// Then we can expose a `dynamic` boolean attribute to control the type.

	for _, domain := range deleted {
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

	for _, domain := range added {
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

	for _, domain := range modified {
		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.UpdateDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()), domain.Name.ValueString())

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

	tflog.Debug(ctx, "Domains", map[string]any{
		"added":    added,
		"deleted":  deleted,
		"modified": modified,
	})
	tflog.Trace(ctx, "Update", map[string]any{"state": fmt.Sprintf("%+v", plan)})
}

// Delete is called when the provider must delete the resource.
// Config values may be read from the DeleteRequest.
//
// If execution completes without error, the framework will automatically call
// DeleteResponse.State.RemoveResource().
func (r *ServiceVCLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *models.ServiceVCLResourceModel

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

	tflog.Trace(ctx, "Delete", map[string]any{"state": fmt.Sprintf("%+v", state)})
}

// Configure includes provider-level data or clients.
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

// ImportState is called when the provider must import the state of a resource instance.
//
// This method must return enough state so the Read method can properly refresh
// the full resource.
//
// If setting an attribute with the import identifier, it is recommended to use
// the ImportStatePassthroughID() call in this method.
// https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource#ImportStatePassthroughID
func (r *ServiceVCLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// TODO: req.ID needs to be checked for format.
	// Typically just a Service ID but can also be <service id>@<service version>
	// If the @<service_version> format is provided, then we need to parse the
	// version and set it into the `version` attribute as well as `last_active`.

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	var state map[string]tftypes.Value
	err := resp.State.Raw.As(&state)
	if err == nil {
		tflog.Debug(ctx, "ImportState", map[string]any{"state": fmt.Sprintf("%+v", state)})
	}
}

// ConfigValidators returns a list of functions which will all be performed during validation.
// https://developer.hashicorp.com/terraform/plugin/framework/resources/validate-configuration#configvalidators-method
func (r ServiceVCLResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("force"),
			path.MatchRoot("reuse"),
		),
	}
}
