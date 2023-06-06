package serviceactivation

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Delete is called when the provider must delete the resource.
// Config values may be read from the DeleteRequest.
//
// If execution completes without error, the framework will automatically call
// DeleteResponse.State.RemoveResource().
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *models.ServiceActivation

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Activate.ValueBool() {
		fmt.Printf("state.Activate.ValueBool(): %+v\n", state.Activate.ValueBool())
	}

	tflog.Trace(ctx, "ACTIVATION Delete", map[string]any{"state": fmt.Sprintf("%+v", state)})
}
