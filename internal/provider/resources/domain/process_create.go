package domain

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Create is called when the provider must create a new resource.
// Config and planned state values should be read from the CreateRequest.
// New state values set on the CreateResponse.
func (r *Resource) Create(
	ctx context.Context,
	req *resource.CreateRequest,
	resp *resource.CreateResponse,
	api helpers.API,
	serviceData *helpers.Service,
) error {
	var domains map[string]models.Domain
	req.Plan.GetAttribute(ctx, path.Root("domains"), &domains)

	for _, domainData := range domains {
		if err := create(ctx, domainData, api, serviceData, resp); err != nil {
			return err
		}
	}

	req.Plan.SetAttribute(ctx, path.Root("domains"), &domains)

	return nil
}

// create is the common behaviour for creating this resource.
func create(
	ctx context.Context,
	domainData models.Domain,
	api helpers.API,
	service *helpers.Service,
	resp *resource.CreateResponse,
) error {
	createErr := errors.New("failed to create domain resource")

	clientReq := api.Client.DomainAPI.CreateDomain(
		api.ClientCtx,
		service.ID,
		service.Version,
	)

	clientReq.Name(domainData.Name.ValueString())

	if !domainData.Comment.IsNull() {
		clientReq.Comment(domainData.Comment.ValueString())
	}

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.CreateDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to create domain, got error: %s", err))
		return createErr
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, helpers.ErrorAPI, map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return createErr
	}

	return nil
}
