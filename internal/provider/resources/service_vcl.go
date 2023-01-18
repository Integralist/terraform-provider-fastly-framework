package resources

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/data"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/interfaces"
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
func NewServiceVCLResource() func() resource.Resource {
	return func() resource.Resource {
		return &ServiceVCLResource{
			nestedResources: []interfaces.Resource{
				NewDomainResource(),
			},
		}
	}
}

// ServiceVCLResource defines the resource implementation.
type ServiceVCLResource struct {
	// client is a preconfigured instance of the Fastly API client.
	client *fastly.APIClient
	// clientCtx contains the user's API token.
	clientCtx context.Context
	// nestedResources is a list of nested nestedResources.
	//
	// NOTE: Terraform doesn't have a concept of nested nestedResources.
	// We're using this terminology because it makes more sense for Fastly.
	// As our 'nested nestedResources' are actually just nested 'attributes'.
	// https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes#nested-attributes
	nestedResources []interfaces.Resource
}

// Metadata should return the full name of the resource.
func (r *ServiceVCLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_vcl"
}

// Schema should return the schema for this resource.
func (r *ServiceVCLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := schemas.Service()

	attrs["default_ttl"] = schema.Int64Attribute{
		Computed:            true,
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
		Computed:            true,
		MarkdownDescription: "Enables serving a stale object if there is an error",
		Optional:            true,
		PlanModifiers: []planmodifier.Bool{
			helpers.BoolDefaultModifier{Default: false},
		},
	}
	attrs["stale_if_error_ttl"] = schema.Int64Attribute{
		Computed:            true,
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
	api := helpers.API{
		Client:    r.client,
		ClientCtx: r.clientCtx,
	}

	serviceID, serviceVersion, err := createService(ctx, req, resp, api)
	if err != nil {
		return
	}

	// IMPORTANT: nestedResources are expected to mutate the plan data.
	for _, nestedResource := range r.nestedResources {
		serviceData := data.Service{
			ID:      serviceID,
			Version: serviceVersion,
		}
		if err := nestedResource.Create(ctx, &req, resp, api, &serviceData); err != nil {
			return
		}
	}

	// Store the planned changes so they can be saved into Terraform state.
	var plan *models.ServiceVCL
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(serviceID)
	plan.Version = types.Int64Value(int64(serviceVersion))
	plan.LastActive = types.Int64Null()

	// NOTE: There is no 'create service settings' API, only 'update'.
	// So even though we're inside the CREATE function, we call updateSettings().
	err = updateSettings(ctx, plan, resp.Diagnostics, api)
	if err != nil {
		return
	}

	if plan.Activate.ValueBool() {
		plan.LastActive = plan.Version

		clientReq := r.client.VersionAPI.ActivateServiceVersion(r.clientCtx, serviceID, serviceVersion)
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly VersionAPI.ActivateServiceVersion error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate service version %d, got error: %s", 1, err))
			return
		}
	}

	// Save the planned changes into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Trace(ctx, "Create", map[string]any{"state": fmt.Sprintf("%+v", plan)})
}

// Read is called when the provider must read resource values in order to update state.
// Planned state values should be read from the ReadRequest.
// New state values set on the ReadResponse.
//
// TODO: How to handle DeletedAt attribute.
// TODO: How to handle service type mismatch when importing.
// TODO: How to handle name/comment which are versionless and don't need `activate`.
func (r *ServiceVCLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Store the prior state (if any) so it can later be mutated and saved back into state.
	var state *models.ServiceVCL
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

	// NOTE: When importing a service there is no prior 'serviceVersion' in the state.
	// So we presume the user wants to import the last active service serviceVersion.
	// Which we retrieve from the GetServiceDetail call.
	var (
		foundActive    bool
		serviceVersion int64
	)
	versions := clientResp.GetVersions()
	for _, version := range versions {
		if version.GetActive() {
			serviceVersion = int64(version.GetNumber())
			foundActive = true
			break
		}
	}

	if !foundActive {
		// Use latest version if the user imports a service with no active versions.
		serviceVersion = int64(versions[0].GetNumber())
	}

	api := helpers.API{
		Client:    r.client,
		ClientCtx: r.clientCtx,
	}

	// IMPORTANT: nestedResources are expected to mutate the plan data.
	for _, nestedResource := range r.nestedResources {
		serviceData := data.Service{
			ID:      clientResp.GetID(),
			Version: int32(serviceVersion),
		}
		if err := nestedResource.Read(ctx, &req, resp, api, &serviceData); err != nil {
			return
		}
	}

	// Refresh the Terraform state data inside the model.
	// As the state is expected to be mutated by nested resources.
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Comment = types.StringValue(clientResp.GetComment())
	state.ID = types.StringValue(clientResp.GetID())
	state.Name = types.StringValue(clientResp.GetName())
	state.Version = types.Int64Value(serviceVersion)
	state.LastActive = types.Int64Value(serviceVersion)

	err = readSettings(ctx, state, resp, api)
	if err != nil {
		return
	}

	// Save the updated state data back into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Trace(ctx, "Read", map[string]any{"state": fmt.Sprintf("%+v", state)})
}

// Update is called to update the state of the resource.
// Config, planned state, and prior state values should be read from the UpdateRequest.
// New state values set on the UpdateResponse.
//
// NOTE: The service attributes (Name, Comment) are 'versionless'.
// Other nested attributes will require a new service version.
func (r *ServiceVCLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// NOTE: The determineChanges() function will mutate the plan model data.
	// This happens from within nestedResources.InspectChanges().
	resourcesChanged, err := determineChanges(ctx, r.nestedResources, &req, resp)
	if err != nil {
		return
	}

	var plan *models.ServiceVCL
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state *models.ServiceVCL
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// NOTE: The plan data doesn't contain computed attributes.
	// So we need to read it from the current state.
	plan.Version = state.Version
	plan.LastActive = state.LastActive

	serviceID := plan.ID.ValueString()
	serviceVersion := int32(plan.Version.ValueInt64())

	api := helpers.API{
		Client:    r.client,
		ClientCtx: r.clientCtx,
	}

	if resourcesChanged {
		// IMPORTANT: We're shadowing the parent scope's serviceVersion variable.
		serviceVersion, err = cloneService(ctx, plan, resp, api, serviceID, serviceVersion)
		if err != nil {
			return
		}
	}

	// IMPORTANT: nestedResources are expected to mutate the plan data.
	// NOTE: Update operation blurs CRUD lines as nested resources also handle create and delete.
	for _, nestedResource := range r.nestedResources {
		if nestedResource.HasChanges() {
			serviceData := data.Service{
				ID:      serviceID,
				Version: serviceVersion,
			}
			if err := nestedResource.Update(ctx, &req, resp, api, &serviceData); err != nil {
				return
			}
		}
	}

	err = updateService(ctx, plan, resp, api, state)
	if err != nil {
		return
	}

	err = updateSettings(ctx, plan, resp.Diagnostics, api)
	if err != nil {
		return
	}

	// Save the planned changes into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if resourcesChanged {
		clientReq := r.client.VersionAPI.ActivateServiceVersion(r.clientCtx, plan.ID.ValueString(), serviceVersion)
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly VersionAPI.ActivateServiceVersion error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate service version %d, got error: %s", 1, err))
			return
		}
	}

	tflog.Trace(ctx, "Update", map[string]any{"state": fmt.Sprintf("%+v", plan)})
}

// Delete is called when the provider must delete the resource.
// Config values may be read from the DeleteRequest.
//
// If execution completes without error, the framework will automatically call
// DeleteResponse.State.RemoveResource().
func (r *ServiceVCLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *models.ServiceVCL

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

func createService(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
	api helpers.API,
) (serviceID string, serviceVersion int32, err error) {
	var plan *models.ServiceVCL

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return "", 0, errors.New("failed to read Terraform plan")
	}

	clientReq := api.Client.ServiceAPI.CreateService(api.ClientCtx)
	clientReq.Comment(plan.Comment.ValueString())
	clientReq.Name(plan.Name.ValueString())
	clientReq.ResourceType("vcl")

	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly ServiceAPI.CreateService error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create service, got error: %s", err))
		return "", 0, err
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return "", 0, fmt.Errorf("failed to create service: %s", httpResp.Status)
	}

	id, ok := clientResp.GetIDOk()
	if !ok {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", "No Service ID was returned")
		return "", 0, errors.New("failed to create service: no Service ID returned")
	}

	versions, ok := clientResp.GetVersionsOk()
	if !ok {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", "No Service versions returned")
		return "", 0, errors.New("failed to create service: no Service versions returned")
	}
	version := versions[0].GetNumber()

	return *id, version, nil
}

func readSettings(ctx context.Context, state *models.ServiceVCL, resp *resource.ReadResponse, api helpers.API) error {
	serviceID := state.ID.ValueString()
	serviceVersion := int32(state.Version.ValueInt64())

	clientReq := api.Client.SettingsAPI.GetServiceSettings(api.ClientCtx, serviceID, serviceVersion)

	readErr := errors.New("failed to read service settings")

	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly SettingsAPI.GetServiceSettings error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read service settings, got error: %s", err))
		return readErr
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return readErr
	}

	if ptr, ok := clientResp.GetGeneralDefaultHostOk(); ok {
		// WARNING: This block of code doesn't work as you might expect because of the Fastly API.
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
		// client could handle whether the value returned was null.
		if !state.DefaultHost.IsNull() {
			state.DefaultHost = types.StringValue(*ptr)
		}
	}
	if ptr, ok := clientResp.GetGeneralDefaultTTLOk(); ok {
		state.DefaultTTL = types.Int64Value(int64(*ptr))
	}
	if ptr, ok := clientResp.GetGeneralStaleIfErrorOk(); ok {
		state.StaleIfError = types.BoolValue(*ptr)
	}
	if ptr, ok := clientResp.GetGeneralStaleIfErrorTTLOk(); ok {
		state.StaleIfErrorTTL = types.Int64Value(int64(*ptr))
	}

	return nil
}

func updateSettings(ctx context.Context, plan *models.ServiceVCL, diags diag.Diagnostics, api helpers.API) error {
	serviceID := plan.ID.ValueString()
	serviceVersion := int32(plan.Version.ValueInt64())

	clientReq := api.Client.SettingsAPI.UpdateServiceSettings(api.ClientCtx, serviceID, serviceVersion)

	if !plan.DefaultHost.IsNull() {
		clientReq.GeneralDefaultHost(plan.DefaultHost.ValueString())
	}
	if !plan.DefaultTTL.IsNull() {
		clientReq.GeneralDefaultTTL(int32(plan.DefaultTTL.ValueInt64()))
	}
	if !plan.StaleIfError.IsNull() {
		clientReq.GeneralStaleIfError(plan.StaleIfError.ValueBool())
	}
	if !plan.StaleIfErrorTTL.IsNull() {
		clientReq.GeneralStaleIfErrorTTL(int32(plan.StaleIfErrorTTL.ValueInt64()))
	}

	createErr := errors.New("failed to set service settings")

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly SettingsAPI.UpdateServiceSettings error", map[string]any{"http_resp": httpResp})
		diags.AddError("Client Error", fmt.Sprintf("Unable to set service settings, got error: %s", err))
		return createErr
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		diags.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return createErr
	}

	return nil
}

func determineChanges(
	ctx context.Context,
	nestedResources []interfaces.Resource,
	req *resource.UpdateRequest,
	resp *resource.UpdateResponse,
) (resourcesChanged bool, err error) {
	// IMPORTANT: We use a counter instead of a bool to avoid unsetting.
	// Because we range over multiple nested attributes, if we had used a boolean
	// then we might find the last item in the loop had no resourcesChanged and we would
	// incorrectly set the boolean to false when prior items DID have resourcesChanged.

	for _, nestedResource := range nestedResources {
		// NOTE: InspectChanges mutates the nested resource.
		// The nestedResource struct has Added, Deleted, Modified fields.
		// These are used by the nestedResource.Update method (called later).
		changed, err := nestedResource.InspectChanges(
			ctx, req, resp, helpers.API{}, &data.Service{},
		)
		if err != nil {
			tflog.Trace(ctx, "Provider error", map[string]any{"error": err})
			resp.Diagnostics.AddError("Provider Error", fmt.Sprintf("InspectChanges failed to detect changes, got error: %s", err))
			return false, err
		}

		if changed {
			resourcesChanged = true
		}
	}

	return resourcesChanged, nil
}

func cloneService(
	ctx context.Context,
	plan *models.ServiceVCL,
	resp *resource.UpdateResponse,
	api helpers.API,
	serviceID string,
	serviceVersion int32,
) (version int32, err error) {
	clientReq := api.Client.VersionAPI.CloneServiceVersion(api.ClientCtx, serviceID, serviceVersion)
	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly VersionAPI.CloneServiceVersion error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to clone service version, got error: %s", err))
		return 0, err
	}
	plan.Version = types.Int64Value(int64(clientResp.GetNumber()))

	// TODO: Figure out what this API does and why we call it?
	clientUpdateServiceVersionReq := api.Client.VersionAPI.UpdateServiceVersion(api.ClientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()))
	clientUpdateServiceVersionResp, httpResp, err := clientUpdateServiceVersionReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly VersionAPI.UpdateServiceVersion error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update service version, got error: %s", err))
		return 0, err
	}

	return clientUpdateServiceVersionResp.GetNumber(), nil
}

func updateService(
	ctx context.Context,
	plan *models.ServiceVCL,
	resp *resource.UpdateResponse,
	api helpers.API,
	state *models.ServiceVCL,
) error {
	// NOTE: UpdateService doesn't take a version because its attributes are versionless.
	// When cloning (see above) we need to call UpdateServiceVersion.
	clientReq := api.Client.ServiceAPI.UpdateService(api.ClientCtx, plan.ID.ValueString())
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
		return err
	}

	return nil
}
