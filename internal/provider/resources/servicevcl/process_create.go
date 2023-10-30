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

// Create is called when the provider must create a new resource.
// Config and planned state values should be read from the CreateRequest.
// New state values set on the CreateResponse.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
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
		serviceData := helpers.Service{
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
	err = updateServiceSettings(ctx, plan, resp.Diagnostics, api)
	if err != nil {
		return
	}

	if plan.Activate.ValueBool() {
		clientReq := r.client.VersionAPI.ActivateServiceVersion(r.clientCtx, serviceID, serviceVersion)
		_, httpResp, err := clientReq.Execute()
		if err != nil {
			tflog.Trace(ctx, "Fastly VersionAPI.ActivateServiceVersion error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to activate service version %d, got error: %s", 1, err))
			return
		}
		defer httpResp.Body.Close()

		// Only set LastActive to Version if we successfully activate the service.
		plan.LastActive = plan.Version
	}

	// Save the planned changes into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Create", map[string]any{"state": fmt.Sprintf("%#v", plan)})
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
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to create service, got error: %s", err))
		return "", 0, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, helpers.ErrorAPI, map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return "", 0, fmt.Errorf("failed to create service: %s", httpResp.Status)
	}

	id, ok := clientResp.GetIDOk()
	if !ok {
		tflog.Trace(ctx, helpers.ErrorAPI, map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, "No Service ID was returned")
		return "", 0, errors.New("failed to create service: no Service ID returned")
	}

	versions, ok := clientResp.GetVersionsOk()
	if !ok {
		tflog.Trace(ctx, helpers.ErrorAPI, map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, "No Service versions returned")
		return "", 0, errors.New("failed to create service: no Service versions returned")
	}
	version := versions[0].GetNumber()

	return *id, version, nil
}
