package servicevcl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/fastly/fastly-go/fastly"
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
	if state == nil {
		tflog.Trace(ctx, helpers.ErrorTerraformPointer, map[string]any{"req": req, "resp": resp})
		resp.Diagnostics.AddError(helpers.ErrorTerraformPointer, "nil pointer after state population")
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
		tflog.Trace(ctx, "Fastly service type error", map[string]any{"http_resp": httpResp, "type": serviceType})
		resp.Diagnostics.AddError(helpers.ErrorUser, fmt.Sprintf("Expected service type %s, got: %s", vclServiceType, serviceType))
		return
	}

	remoteServiceVersion, err := readServiceVersion(state, clientResp)
	if err != nil {
		tflog.Trace(ctx, "Fastly service version identification error", map[string]any{"state": state, "service_details": clientResp, "error": err})
		resp.Diagnostics.AddError(helpers.ErrorUnknown, err.Error())
		return
	}

	// If the user has indicated they want their service to be 'active', then we
	// presume when refreshing the state that we should be dealing with a service
	// version that is active. If the prior state has a `version` field that
	// doesn't match the current latest active version, then this suggests that
	// the service versions have drifted outside of Terraform.
	//
	// e.g. a user has reverted the service version to another version via the UI.
	//
	// In this scenario, we'll set `force_refresh=true` so that the nested
	// resources will call the Fastly API to get updated state information.
	if state.Activate.ValueBool() && state.Version != types.Int64Value(remoteServiceVersion) {
		state.ForceRefresh = types.BoolValue(true)
	}

	api := helpers.API{
		Client:    r.client,
		ClientCtx: r.clientCtx,
	}

	// IMPORTANT: nestedResources are expected to mutate the `req` plan data.
	//
	// We really should modify the `state` variable instead.
	// The reason we don't do this is for interface consistency.
	// i.e. The interfaces.Resource.Read() can have a consistent type.
	// This is because the `state` variable type can change based on the resource.
	// e.g. `models.ServiceVCL` or `models.ServiceCompute`.
	// See `readSettings()` for an example of directly modifying `state`.
	for _, nestedResource := range r.nestedResources {
		serviceData := helpers.Service{
			ID:      clientResp.GetID(),
			Version: int32(remoteServiceVersion),
		}
		if err := nestedResource.Read(ctx, &req, resp, api, &serviceData); err != nil {
			return
		}
	}

	// Sync the Terraform `state` data.
	// As the `req` state is expected to be mutated by nested resources.
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	setServiceState(state, clientResp, remoteServiceVersion)

	err = readServiceSettings(ctx, remoteServiceVersion, state, resp, api)
	if err != nil {
		return
	}

	// To ensure nested resources don't continue to call the Fastly API to
	// refresh the internal Terraform state, we set `imported`/`force_refresh`
	// back to false.
	//
	// `force_refresh` is set to true earlier in this method.
	// `imported` is set to true when `ImportState()` is called in ./resource.go
	//
	// We do this because it's slow and expensive to refresh the state for every
	// nested resource if they've not even been defined in the user's TF config.
	// But during an import we DO want to refresh all the state because we can't
	// know up front what nested resources should exist.
	state.ForceRefresh = types.BoolValue(false)
	state.Imported = types.BoolValue(false)

	// Save the final `state` data back into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read", map[string]any{"state": fmt.Sprintf("%#v", state)})
}

// readServiceVersion returns the service version.
//
// The returned values depends on if we're in an import scenario.
//
// When importing a service there might be no prior `version` in state.
// If the user imports using the `ID@VERSION` syntax, then there will be.
// This is because `ImportState()` in ./resource.go makes sure it's set.
//
// So we check if `imported` is set and if the `version` attribute is not null.
// If these conditions are true we'll check the specified version exists.
// (see `versionFromImport()` for details).
//
// If the conditions aren't met, then we'll call the Fastly API to get all
// available service versions, and then we'll figure out which version we want
// to return (see `versionFromRemote()` for details).
func readServiceVersion(state *models.ServiceVCL, serviceDetailsResp *fastly.ServiceDetail) (serviceVersion int64, err error) {
	if state.Imported.ValueBool() && !state.Version.IsNull() {
		serviceVersion, err = versionFromImport(state, serviceDetailsResp)
	} else {
		serviceVersion, err = versionFromAttr(state, serviceDetailsResp)
	}
	return serviceVersion, err
}

// versionFromImport returns import specified service version.
// It will validate the version specified actually exists remotely.
func versionFromImport(state *models.ServiceVCL, serviceDetailsResp *fastly.ServiceDetail) (serviceVersion int64, err error) {
	serviceVersion = state.Version.ValueInt64() // whatever version the user specified in their import
	versions := serviceDetailsResp.GetVersions()
	var foundVersion bool
	for _, version := range versions {
		if int64(version.GetNumber()) == serviceVersion {
			foundVersion = true
			break
		}
	}
	if !foundVersion {
		err = fmt.Errorf("failed to find version '%d' remotely", serviceVersion)
	}
	return serviceVersion, err
}

// versionFromAttr returns the service version based on `activate` attribute.
// If `activate=true`, then we return the latest 'active' service version.
// If `activate=false` we return the latest version. This allows state drift.
func versionFromAttr(state *models.ServiceVCL, serviceDetailsResp *fastly.ServiceDetail) (serviceVersion int64, err error) {
	versions := serviceDetailsResp.GetVersions()
	size := len(versions)
	switch {
	case size == 0:
		err = errors.New("failed to find any service versions remotely")
	case state.Activate.IsNull():
		fallthrough // when importing `activate` doesn't have its default value set so we default to importing the latest 'active' version.
	case state.Activate.ValueBool():
		var foundVersion bool
		for _, version := range versions {
			if version.GetActive() {
				serviceVersion = int64(version.GetNumber())
				foundVersion = true
				break
			}
		}
		if !foundVersion {
			// If we're importing a service, then we don't have `activate` value.
			// So if there's no active version to use, fallback the latest version.
			if state.Imported.ValueBool() {
				serviceVersion = getLatestServiceVersion(size-1, versions)
			} else {
				err = errors.New("failed to find active version remotely")
			}
		}
	default:
		// If `activate=false` then we expect state drift and will pull in the
		// latest version available (regardless of if it's active or not).
		serviceVersion = getLatestServiceVersion(size-1, versions)
	}
	return serviceVersion, err
}

func getLatestServiceVersion(i int, versions []fastly.SchemasVersionResponse) int64 {
	return int64(versions[i].GetNumber())
}

// setServiceState mutates the resource state with service data from the API.
func setServiceState(state *models.ServiceVCL, clientResp *fastly.ServiceDetail, remoteServiceVersion int64) {
	state.Comment = types.StringValue(clientResp.GetComment())
	state.ID = types.StringValue(clientResp.GetID())
	state.Name = types.StringValue(clientResp.GetName())
	state.Version = types.Int64Value(remoteServiceVersion)

	// We set `last_active` to align with `version` only if `activate=true`.
	// We only expect `version` to drift from `last_active` if `activate=false`.
	if state.Activate.ValueBool() {
		state.LastActive = types.Int64Value(remoteServiceVersion)
	}
}

func readServiceSettings(ctx context.Context, serviceVersion int64, state *models.ServiceVCL, resp *resource.ReadResponse, api helpers.API) error {
	serviceID := state.ID.ValueString()
	clientReq := api.Client.SettingsAPI.GetServiceSettings(api.ClientCtx, serviceID, int32(serviceVersion))
	readErr := errors.New("failed to read service settings")

	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly SettingsAPI.GetServiceSettings error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to read service settings, got error: %s", err))
		return readErr
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, helpers.ErrorAPI, map[string]any{"http_resp": httpResp})
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
