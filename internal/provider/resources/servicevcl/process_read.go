package servicevcl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Read is called when the provider must read resource values in order to update state.
// Planned state values should be read from the ReadRequest.
// New state values set on the ReadResponse.
//
// TODO: How to handle name/comment which are versionless and don't need `activate`.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
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
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to retrieve service details, got error: %s", err))
		return
	}
	defer httpResp.Body.Close()

	// Check if the service has been deleted outside of Terraform.
	// And if so we'll just return.
	if t, ok := clientResp.GetDeletedAtOk(); ok && t != nil {
		tflog.Trace(ctx, "Fastly ServiceAPI.GetDeletedAtOk", map[string]any{"deleted_at": t, "state": state})
		resp.State.RemoveResource(ctx)
		return
	}

	// Avoid issue with service type mismatch (only relevant when importing).
	serviceType := clientResp.GetType()
	vclServiceType := helpers.ServiceTypeVCL.String()
	if serviceType != vclServiceType {
		tflog.Debug(ctx, "Fastly service type error", map[string]any{"http_resp": httpResp, "type": serviceType})
		resp.Diagnostics.AddError(helpers.ErrorUser, fmt.Sprintf("Expected service type %s, got: %s", vclServiceType, serviceType))
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
		serviceData := helpers.Service{
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

	tflog.Trace(ctx, "Read", map[string]any{"state": fmt.Sprintf("%#v", state)})
}

func readSettings(ctx context.Context, state *models.ServiceVCL, resp *resource.ReadResponse, api helpers.API) error {
	serviceID := state.ID.ValueString()
	serviceVersion := int32(state.Version.ValueInt64())

	clientReq := api.Client.SettingsAPI.GetServiceSettings(api.ClientCtx, serviceID, serviceVersion)

	readErr := errors.New("failed to read service settings")

	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly SettingsAPI.GetServiceSettings error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to read service settings, got error: %s", err))
		return readErr
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
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
		//
		// FIXME: How should we handle 'default values'?
		// I presume we need to assign a default via attribute schema to avoid
		// conflicts within Terraform plan diffs.
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
