package domain

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Update is called to update the state of the resource.
// Config, planned state, and prior state values should be read from the UpdateRequest.
// New state values set on the UpdateResponse.
func (r *Resource) Update(
	ctx context.Context,
	_ *resource.UpdateRequest,
	resp *resource.UpdateResponse,
	api helpers.API,
	serviceData *helpers.Service,
) error {
	// IMPORTANT: We need to delete, then add, then update.
	// Some Fastly resources (like snippets) must have unique names.
	// If a user tries to switch from dynamicsnippet to snippet, and we don't
	// delete the resource first before creating the new one, then the Fastly API
	// will return an error and indicate that we have a conflict.
	//
	// FIXME: VCL Snippets need to be consolidated into a single type.
	// In the current Fastly provider there is a race condition bug.
	// https://github.com/fastly/terraform-provider-fastly/issues/628#issuecomment-1372477539
	// Which is based on the fact that snippets are two separate types.
	// We should make them a single type (as the API is one endpoint).
	// Then we can expose a `dynamic` boolean attribute to control the type.

	for _, domainData := range r.Deleted {
		if err := deleted(ctx, api, serviceData, domainData, resp); err != nil {
			return err
		}
	}

	for _, domainData := range r.Added {
		if err := added(ctx, api, serviceData, domainData, resp); err != nil {
			return err
		}
	}

	for _, domainData := range r.Modified {
		if err := modified(ctx, api, serviceData, domainData, resp); err != nil {
			return err
		}
	}

	r.Added = nil
	r.Deleted = nil
	r.Modified = nil
	r.Changed = false

	return nil
}

func deleted(
	ctx context.Context,
	api helpers.API,
	serviceData *helpers.Service,
	domainData models.Domain,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.DeleteDomain(api.ClientCtx, serviceData.ID, serviceData.Version, domainData.Name.ValueString())

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.DeleteDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to delete domain, got error: %s", err))
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return err
	}

	return nil
}

func added(
	ctx context.Context,
	api helpers.API,
	serviceData *helpers.Service,
	domainData models.Domain,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.CreateDomain(api.ClientCtx, serviceData.ID, serviceData.Version)
	clientReq.Name(domainData.Name.ValueString())

	if !domainData.Comment.IsNull() {
		clientReq.Comment(domainData.Comment.ValueString())
	}

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.CreateDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to create domain, got error: %s", err))
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return err
	}

	return nil
}

func modified(
	ctx context.Context,
	api helpers.API,
	serviceData *helpers.Service,
	domainData models.Domain,
	resp *resource.UpdateResponse,
) error {
	domainNameParam := domainData.Name.ValueString()
	namePast := domainData.NamePast.ValueString()
	if namePast != "" {
		domainNameParam = namePast
	}

	clientReq := api.Client.DomainAPI.UpdateDomain(api.ClientCtx, serviceData.ID, serviceData.Version, domainNameParam)
	if !domainData.Comment.IsNull() {
		clientReq.Comment(domainData.Comment.ValueString())
	}
	clientReq.Name(domainData.Name.ValueString())

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.UpdateDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPIClient, fmt.Sprintf("Unable to update domain, got error: %s", err))
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError(helpers.ErrorAPI, fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return err
	}

	return nil
}
