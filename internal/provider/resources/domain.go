package resources

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/data"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/interfaces"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// NewDomainResource returns a new resource entity.
func NewDomainResource() interfaces.Resource {
	return &DomainResource{}
}

// DomainResource represents a Fastly entity.
type DomainResource struct {
	// Added represents any new resources.
	Added map[string]models.Domain
	// Deleted represents any deleted resources.
	Deleted map[string]models.Domain
	// Modified represents any modified resources.
	Modified map[string]models.Domain
	// Changed indicates if the resource has changes.
	Changed bool
}

// Create is called when the provider must create a new resource.
// Config and planned state values should be read from the CreateRequest.
// New state values set on the CreateResponse.
func (r *DomainResource) Create(
	ctx context.Context,
	req *resource.CreateRequest,
	resp *resource.CreateResponse,
	api helpers.API,
	serviceData *data.Service,
) error {
	var domains map[string]models.Domain
	req.Plan.GetAttribute(ctx, path.Root("domains"), &domains)

	for domainName, domainData := range domains {
		if err := create(ctx, domainName, domainData, api, serviceData, resp); err != nil {
			return err
		}
	}

	req.Plan.SetAttribute(ctx, path.Root("domains"), &domains)

	return nil
}

// Read is called when the provider must read resource values in order to update state.
// Planned state values should be read from the ReadRequest.
// New state values set on the ReadResponse.
func (r *DomainResource) Read(
	ctx context.Context,
	req *resource.ReadRequest,
	resp *resource.ReadResponse,
	api helpers.API,
	serviceData *data.Service,
) error {
	var domains map[string]models.Domain
	req.State.GetAttribute(ctx, path.Root("domains"), &domains)

	remoteDomains, err := read(ctx, domains, api, serviceData, resp)
	if err != nil {
		return err
	}

	req.State.SetAttribute(ctx, path.Root("domains"), &remoteDomains)

	return nil
}

// Update is called to update the state of the resource.
// Config, planned state, and prior state values should be read from the UpdateRequest.
// New state values set on the UpdateResponse.
func (r *DomainResource) Update(
	ctx context.Context,
	_ *resource.UpdateRequest,
	resp *resource.UpdateResponse,
	api helpers.API,
	serviceData *data.Service,
) error {
	// IMPORTANT: We need to delete, then add, then update.
	// Some Fastly resources (like snippets) must have unique names.
	// If a user tries to switch from dynamicsnippet to snippet, and we don't
	// delete the resource first before creating the new one, then the Fastly API
	// will return an error and indicate that we have a conflict.
	//
	// FIXME: In the current Fastly provider there is a race condition bug.
	// https://github.com/fastly/terraform-provider-fastly/issues/628#issuecomment-1372477539
	// Which is based on the fact that snippets are two separate types.
	// We should make them a single type (as the API is one endpoint).
	// Then we can expose a `dynamic` boolean attribute to control the type.

	for domainName := range r.Deleted {
		if err := updateDeleted(ctx, api, serviceData, domainName, resp); err != nil {
			return err
		}
	}

	for domainName, domainData := range r.Added {
		if err := updateAdded(ctx, api, serviceData, domainName, domainData, resp); err != nil {
			return err
		}
	}

	for domainName, domainData := range r.Modified {
		if err := updateModified(ctx, api, serviceData, domainName, domainData, resp); err != nil {
			return err
		}
	}

	r.Added = nil
	r.Deleted = nil
	r.Modified = nil
	r.Changed = false

	return nil
}

// InspectChanges checks for configuration changes and persists to data model.
func (r *DomainResource) InspectChanges(
	ctx context.Context,
	req *resource.UpdateRequest,
	_ *resource.UpdateResponse,
	_ helpers.API,
	_ *data.Service,
) (bool, error) {
	var planDomains map[string]models.Domain
	var stateDomains map[string]models.Domain

	req.Plan.GetAttribute(ctx, path.Root("domains"), &planDomains)
	req.State.GetAttribute(ctx, path.Root("domains"), &stateDomains)

	r.Changed, r.Added, r.Deleted, r.Modified = inspectChanges(planDomains, stateDomains)

	tflog.Debug(context.Background(), "Domains", map[string]any{
		"added":    r.Added,
		"deleted":  r.Deleted,
		"modified": r.Modified,
		"changed":  r.Changed,
	})

	// NOTE: the inspectChanges() function mutates the plan domains with an ID.
	req.Plan.SetAttribute(ctx, path.Root("domains"), &planDomains)

	return r.Changed, nil
}

// HasChanges indicates if the nested resource contains configuration changes.
func (r *DomainResource) HasChanges() bool {
	return r.Changed
}

// create is the common behaviour for creating this resource.
func create(
	ctx context.Context,
	domainName string,
	domainData models.Domain,
	api helpers.API,
	service *data.Service,
	resp *resource.CreateResponse,
) error {
	createErr := errors.New("failed to create domain resource")

	clientReq := api.Client.DomainAPI.CreateDomain(
		api.ClientCtx,
		service.ID,
		service.Version,
	)

	clientReq.Name(domainName)

	if !domainData.Comment.IsNull() {
		clientReq.Comment(domainData.Comment.ValueString())
	}

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.CreateDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create domain, got error: %s", err))
		return createErr
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return createErr
	}

	return nil
}

func read(
	ctx context.Context,
	stateDomains map[string]models.Domain,
	api helpers.API,
	service *data.Service,
	resp *resource.ReadResponse,
) (map[string]models.Domain, error) {
	clientReq := api.Client.DomainAPI.ListDomains(
		api.ClientCtx,
		service.ID,
		service.Version,
	)

	clientResp, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.ListDomains error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list domains, got error: %s", err))
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return nil, err
	}

	remoteDomains := make(map[string]models.Domain)

	for _, remoteDomain := range clientResp {
		remoteDomainName := remoteDomain.GetName()
		remoteDomainData := models.Domain{}

		// NOTE: We call the Ok variant of the API so we can check if value was set.
		// WARNING: The code doesn't work as you might expect because of the Fastly API.
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
		// client could handle whether the value returned was null. So I've left the
		// conditional logic here but have added to the else statement additional
		// logic for working around the issue with the Fastly API response.
		if v, ok := remoteDomain.GetCommentOk(); ok {
			// Set comment to whatever is returned by the API (could be an empty
			// string and that might be because the user set that explicitly or it
			// could be because it was never set and the API is just returning an
			// empty string as a default value).
			remoteDomainData.Comment = types.StringValue(*v)

			// We need to check if the user config has set the comment.
			// If not, then we'll again set the value to null to avoid a plan diff.
			// See the above WARNING for the details.
			for stateDomainName, stateDomainData := range stateDomains {
				if stateDomainName == remoteDomainName {
					if stateDomainData.Comment.IsNull() {
						remoteDomainData.Comment = types.StringNull()
					}
				}
			}

			// Domains is a required attribute, so if there is a length of zero, then
			// we know that the Read method was called after an import (as an import
			// only sets the Service ID). This means we can't check the prior state to
			// see if the user had configured a value for the comment.
			//
			// WARNING: The domain comment logic for import scenarios is fragile.
			//
			// The problem we have is that a user can set an empty string as a comment
			// value to the Fastly API. This means when importing a service, as we
			// have no prior state to compare with, we can't tell if the value
			// returned by the Fastly API is an empty string because the comment was
			// never set by the user (and the API defaults to returning an empty
			// string) or if it's an empty string because the user's service actually
			// had it set explicitly to be an empty string. In reality, it's very
			// unlikely that a user is going to configure an empty string for the
			// domain comment (they'll more likely just omit the attribute). So we'll
			// presume that if we're in an 'import' scenario and the comment value is
			// an empty string, that we should set the comment attribute to null.
			if len(stateDomains) == 0 && *v == "" {
				remoteDomainData.Comment = types.StringNull()
			}
		} else {
			remoteDomainData.Comment = types.StringNull()
		}

		// NOTE: It's highly unlikely a domain would have no name.
		// But safer to just avoid accidentally setting a map key to an empty string.
		if remoteDomainName == "" {
			tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("API Error", "No domain name set in API response")
			return nil, err
		}

		remoteDomains[remoteDomainName] = remoteDomainData
	}

	return remoteDomains, nil
}

func updateDeleted(
	ctx context.Context,
	api helpers.API,
	serviceData *data.Service,
	domainName string,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.DeleteDomain(api.ClientCtx, serviceData.ID, serviceData.Version, domainName)

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.DeleteDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete domain, got error: %s", err))
		return err
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return err
	}

	return nil
}

func updateAdded(
	ctx context.Context,
	api helpers.API,
	serviceData *data.Service,
	domainName string,
	domainData models.Domain,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.CreateDomain(api.ClientCtx, serviceData.ID, serviceData.Version)
	clientReq.Name(domainName)

	if !domainData.Comment.IsNull() {
		clientReq.Comment(domainData.Comment.ValueString())
	}

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.CreateDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create domain, got error: %s", err))
		return err
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return err
	}

	return nil
}

func updateModified(
	ctx context.Context,
	api helpers.API,
	serviceData *data.Service,
	domainName string,
	domainData models.Domain,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.UpdateDomain(api.ClientCtx, serviceData.ID, serviceData.Version, domainName)

	if !domainData.Comment.IsNull() {
		clientReq.Comment(domainData.Comment.ValueString())
	}

	// NOTE: We don't bother to check/update the domain's Name field.
	// This is because if the name of the domain has changed, then that means
	// the a new domain will be added and the original domain deleted. Thus,
	// we'll only have a domain as 'modified' if the Comment field was modified.

	_, httpResp, err := clientReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.UpdateDomain error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update domain, got error: %s", err))
		return err
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return err
	}

	return nil
}

// MODIFIED:
// If a plan domain name matches a state domain name, then it's been modified.
//
// ADDED:
// If a plan domain name doesn't exist in the state, then it's a new domain.
//
// DELETED:
// If a state domain name doesn't exist in the plan, then it's a deleted domain.
//
// TODO: Figure out, now we're using a map type, can we abstract this logic.
// So it's useful across multiple resources (as long as they're all maps too).
func inspectChanges(planDomains, stateDomains map[string]models.Domain) (changed bool, added, deleted, modified map[string]models.Domain) {
	added = make(map[string]models.Domain)
	modified = make(map[string]models.Domain)
	deleted = make(map[string]models.Domain)

	for planDomainName, planDomainData := range planDomains {
		var foundDomain bool

		for stateDomainName, stateDomainData := range stateDomains {
			if planDomainName == stateDomainName {
				foundDomain = true
				if !planDomainData.Comment.Equal(stateDomainData.Comment) {
					changed = true
					modified[planDomainName] = planDomainData
				}
				break
			}
		}

		if !foundDomain {
			changed = true
			added[planDomainName] = planDomainData
		}
	}

	for stateDomainName, stateDomainData := range stateDomains {
		var foundDomain bool
		for planDomainName := range planDomains {
			if planDomainName == stateDomainName {
				foundDomain = true
				break
			}
		}

		if !foundDomain {
			changed = true
			deleted[stateDomainName] = stateDomainData
		}
	}

	return changed, added, deleted, modified
}
