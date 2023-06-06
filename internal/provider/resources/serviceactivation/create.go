package serviceactivation

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Create is called when the provider must create a new resource.
// Config and planned state values should be read from the CreateRequest.
// New state values set on the CreateResponse.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	api := helpers.API{
		Client:    r.client,
		ClientCtx: r.clientCtx,
	}
	fmt.Printf("api client: %+v\n", api)

	// TODO: Create the resource that will handle service activation.

	// Store the planned changes so they can be saved into Terraform state.
	var plan *models.ServiceActivation
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Activate.ValueBool() {
		fmt.Printf("plan.Activate.ValueBool(): %+v\n", plan.Activate.ValueBool())
		// if err != nil {
		// 	tflog.Trace(ctx, "Fastly VersionAPI.ActivateServiceVersion error", map[string]any{"http_resp": httpResp})
		// 	resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate service version %d, got error: %s", 1, err))
		// 	return
		// }
	}

	// Save the planned changes into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Trace(ctx, "ACTIVATION Create", map[string]any{"state": fmt.Sprintf("%+v", plan)})
}
