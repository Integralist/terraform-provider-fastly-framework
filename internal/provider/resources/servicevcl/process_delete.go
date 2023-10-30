package servicevcl

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Delete is called when the provider must delete the resource.
// Config values may be read from the DeleteRequest.
//
// If execution completes without error, the framework will automatically call
// DeleteResponse.State.RemoveResource().
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *models.ServiceVCL

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ForceDestroy.ValueBool() || state.Reuse.ValueBool() {
		clientReq := r.client.ServiceAPI.GetServiceDetail(r.clientCtx, state.ID.ValueString())
		clientResp, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly ServiceAPI.GetServiceDetail error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to retrieve service details, got error: %s", err))
			return
		}
		defer httpResp.Body.Close()

		// Service was deleted outside of Terraform.
		if deletedAt, _ := clientResp.GetDeletedAtOk(); deletedAt != nil {
			return
		}

		var activeVersion int32
		if clientResp.GetActiveVersion().Number != nil {
			activeVersion = *clientResp.GetActiveVersion().Number
		}

		if activeVersion != 0 {
			clientReq := r.client.VersionAPI.DeactivateServiceVersion(r.clientCtx, state.ID.ValueString(), activeVersion)
			_, httpResp, err := clientReq.Execute()
			if err != nil {
				tflog.Trace(ctx, "Fastly VersionAPI.DeactivateServiceVersion error", map[string]any{"http_resp": httpResp})
				resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to deactivate service version %d, got error: %s", activeVersion, err))
				return
			}
			defer httpResp.Body.Close()
		}
	}

	if !state.Reuse.ValueBool() {
		clientReq := r.client.ServiceAPI.DeleteService(r.clientCtx, state.ID.ValueString())
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly ServiceAPI.DeleteService error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to delete service, got error: %s", err))
			return
		}
		defer httpResp.Body.Close()
	}

	tflog.Debug(ctx, "Delete", map[string]any{"state": fmt.Sprintf("%#v", state)})
}
