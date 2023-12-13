package servicevcl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/interfaces"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Update is called to update the state of the resource.
// Config, planned state, and prior state values should be read from the UpdateRequest.
// New state values set on the UpdateResponse.
func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	nestedResourcesChanged, err := determineChangesInNestedResources(ctx, r.nestedResources, &req, resp)
	if err != nil {
		return
	}

	var plan *models.ServiceVCL
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan == nil {
		tflog.Trace(ctx, helpers.ErrorTerraformPointer, map[string]any{"req": req, "resp": resp})
		resp.Diagnostics.AddError(helpers.ErrorTerraformPointer, "nil pointer after plan population")
		return
	}

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

	if nestedResourcesChanged {
		clonedServiceVersion, err := cloneService(ctx, resp, api, serviceID, serviceVersion)
		if err != nil {
			return
		}
		plan.Version = types.Int64Value(int64(clonedServiceVersion))
		serviceVersion = clonedServiceVersion
	}

	// IMPORTANT: nestedResources are expected to mutate the plan data.
	// NOTE: Update operation blurs CRUD lines as nested resources also handle create and delete.
	for _, nestedResource := range r.nestedResources {
		if nestedResource.HasChanges() {
			serviceData := helpers.Service{
				ID:      serviceID,
				Version: serviceVersion,
			}
			if err := nestedResource.Update(ctx, &req, resp, api, &serviceData); err != nil {
				return
			}
		}
	}

	err = updateServiceSettings(ctx, plan, resp.Diagnostics, api)
	if err != nil {
		return
	}

	if nestedResourcesChanged && plan.Activate.ValueBool() {
		latestVersion, err := activateService(ctx, plan.ID.ValueString(), serviceVersion, r, resp)
		if err != nil {
			return
		}
		plan.LastActive = types.Int64Value(latestVersion)
	}

	// NOTE: The service attributes (Name, Comment) are 'versionless'.
	// In the old Terraform provider implementation we only updated if `activate`
	// was set to `true` but it's unclear why as recent testing shows that it
	// works regardless of whether the service is active or not.
	err = updateServiceAttributes(ctx, plan, resp, api, state)
	if err != nil {
		return
	}

	// Save the planned changes into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Update", map[string]any{"state": fmt.Sprintf("%#v", plan)})
}

func updateServiceSettings(ctx context.Context, plan *models.ServiceVCL, diags diag.Diagnostics, api helpers.API) error {
	if plan == nil {
		return fmt.Errorf("unexpected nil for pointer argument type: %T", plan)
	}

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
		diags.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to set service settings, got error: %s", err))
		return createErr
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, helpers.ErrorAPI, map[string]any{"http_resp": httpResp})
		diags.AddError(helpers.ErrorAPI, fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return createErr
	}

	return nil
}

// activateService activates the service and updates the plan's LastActive.
func activateService(
	ctx context.Context,
	serviceID string,
	serviceVersion int32,
	r *Resource,
	resp *resource.UpdateResponse,
) (int64, error) {
	clientReq := r.client.VersionAPI.ActivateServiceVersion(r.clientCtx, serviceID, serviceVersion)
	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly VersionAPI.ActivateServiceVersion error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to activate service version %d, got error: %s", 1, err))
		return 0, err
	}
	defer httpResp.Body.Close()
	return int64(clientResp.GetNumber()), nil
}

func determineChangesInNestedResources(
	ctx context.Context,
	nestedResources []interfaces.Resource,
	req *resource.UpdateRequest,
	resp *resource.UpdateResponse,
) (resourcesChanged bool, err error) {
	for _, nestedResource := range nestedResources {
		changed, err := nestedResource.InspectChanges(
			ctx, req, resp, helpers.API{}, &helpers.Service{},
		)
		if err != nil {
			tflog.Trace(ctx, "Provider error", map[string]any{"error": err})
			resp.Diagnostics.AddError(helpers.ErrorProvider, fmt.Sprintf("InspectChanges failed to detect changes, got error: %s", err))
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
	resp *resource.UpdateResponse,
	api helpers.API,
	serviceID string,
	serviceVersion int32,
) (version int32, err error) {
	clientReq := api.Client.VersionAPI.CloneServiceVersion(api.ClientCtx, serviceID, serviceVersion)
	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly VersionAPI.CloneServiceVersion error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to clone service version, got error: %s", err))
		return 0, err
	}
	defer httpResp.Body.Close()
	return clientResp.GetNumber(), nil
}

func updateServiceAttributes(
	ctx context.Context,
	plan *models.ServiceVCL,
	resp *resource.UpdateResponse,
	api helpers.API,
	state *models.ServiceVCL,
) error {
	if plan == nil || resp == nil || state == nil {
		return errors.New("unexpected nil for pointer argument type")
	}

	// NOTE: UpdateService doesn't take a version because its attributes are versionless.
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
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to update service, got error: %s", err))
		return err
	}
	defer httpResp.Body.Close()

	return nil
}
