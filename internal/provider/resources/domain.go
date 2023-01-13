package resources

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/data"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/interfaces"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// NewDomainResource returns a new resource entity.
func NewDomainResource() interfaces.Resource {
	return &DomainResource{
		Type: enums.Domain,
	}
}

// DomainResource represents a Fastly entity.
type DomainResource struct {
	// Type is the nested resource type within the Fastly service.
	Type enums.NestedType
	// Added represents any new resources.
	Added []models.Domain
	// Deleted represents any deleted resources.
	Deleted []models.Domain
	// Modified represents any modified resources.
	Modified []models.Domain
	// Changed indicates if the resource has changes.
	Changed bool
}

// GetType returns the nested resource type (e.g. enums.Domain)
func (r *DomainResource) GetType() enums.NestedType {
	return r.Type
}

// Create is called when the provider must create a new resource.
// Config and planned state values should be read from the CreateRequest.
// New state values set on the CreateResponse.
func (r *DomainResource) Create(
	ctx context.Context,
	req *resource.CreateRequest,
	resp *resource.CreateResponse,
	api helpers.API,
	resourceData *data.Resource,
) error {
	var domains []models.Domain
	req.Plan.GetAttribute(ctx, path.Root("domains"), &domains)

	for i := range domains {
		domain := &domains[i]
		if err := create(ctx, domain, api, resourceData, resp); err != nil {
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
	resourceData *data.Resource,
) error {
	var domains []models.Domain
	req.State.GetAttribute(ctx, path.Root("domains"), &domains)

	remoteDomains, err := read(ctx, domains, api, resourceData, resp)
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
	req *resource.UpdateRequest,
	resp *resource.UpdateResponse,
	api helpers.API,
	resourceData *data.Resource,
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

	for _, domain := range r.Deleted {
		if err := updateDeleted(ctx, api, resourceData, domain, resp); err != nil {
			return err
		}
	}

	for _, domain := range r.Added {
		if err := updateAdded(ctx, api, resourceData, domain, resp); err != nil {
			return err
		}
	}

	for _, domain := range r.Modified {
		if err := updateModified(ctx, api, resourceData, domain, resp); err != nil {
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
func (r *DomainResource) InspectChanges(resourceData *data.Resource) (bool, error) {
	plan := resourceData.Plan
	state := resourceData.State

	switch resourceData.Type {
	case enums.Compute:
	// ...
	case enums.VCL:
		planData, ok := plan.(*models.ServiceVCL)
		if !ok {
			return false, fmt.Errorf("unable to convert %T into the expected model type", plan)
		}
		stateData, ok := state.(*models.ServiceVCL)
		if !ok {
			return false, fmt.Errorf("unable to convert %T into the expected model type", state)
		}

		r.Changed, r.Added, r.Deleted, r.Modified = inspectChanges(planData.Domains, stateData.Domains)

		tflog.Debug(context.Background(), "Domains", map[string]any{
			"added":    r.Added,
			"deleted":  r.Deleted,
			"modified": r.Modified,
			"changed":  r.Changed,
		})

		return r.Changed, nil
	}

	return false, fmt.Errorf("unrecognised resource data type: %+v", resourceData.Type)
}

// HasChanges indicates if the nested resource contains configuration changes.
func (r *DomainResource) HasChanges() bool {
	return r.Changed
}

// create is the common behaviour for creating this resource.
func create(
	ctx context.Context,
	domain *models.Domain,
	api helpers.API,
	service *data.Resource,
	resp *resource.CreateResponse,
) error {
	createErr := errors.New("failed to create domain resource")

	if domain.ID.IsUnknown() {
		// NOTE: We create a consistent hash of the domain name for the ID.
		// Originally I used github.com/google/uuid but realised it would be more
		// appropriate to use a hash of the domain name.
		digest := sha256.Sum256([]byte(domain.Name.ValueString()))
		domain.ID = types.StringValue(fmt.Sprintf("%x", digest))
	}

	// TODO: Check if the version we have is correct.
	// e.g. should it be latest 'active' or just latest version?
	// It should depend on `activate` field but also whether the service pre-exists.
	// The service might exist if it was imported or a secondary config run.
	clientReq := api.Client.DomainAPI.CreateDomain(
		api.ClientCtx,
		service.ServiceID,
		service.ServiceVersion,
	)

	if !domain.Comment.IsNull() {
		clientReq.Comment(domain.Comment.ValueString())
	}

	if !domain.Name.IsNull() {
		clientReq.Name(domain.Name.ValueString())
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
	domains []models.Domain,
	api helpers.API,
	service *data.Resource,
	resp *resource.ReadResponse,
) ([]models.Domain, error) {
	clientReq := api.Client.DomainAPI.ListDomains(
		api.ClientCtx,
		service.ServiceID,
		service.ServiceVersion,
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

	var remoteDomains []models.Domain

	for _, domain := range clientResp {
		domainName := domain.GetName()
		digest := sha256.Sum256([]byte(domainName))

		sd := models.Domain{
			ID: types.StringValue(fmt.Sprintf("%x", digest)),
		}

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
		if v, ok := domain.GetCommentOk(); ok {
			// Set comment to whatever is returned by the API (could be an empty
			// string and that might be because the user set that explicitly or it
			// could be because it was never set and the API is just returning an
			// empty string as a default value).
			sd.Comment = types.StringValue(*v)

			// We need to check if the user config has set the comment.
			// If not, then we'll again set the value to null to avoid a plan diff.
			// See the above WARNING for the details.
			for _, stateDomain := range domains {
				if stateDomain.Name.ValueString() == domainName {
					if stateDomain.Comment.IsNull() {
						sd.Comment = types.StringNull()
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
			if len(domains) == 0 && *v == "" {
				sd.Comment = types.StringNull()
			}
		} else {
			sd.Comment = types.StringNull()
		}

		if v, ok := domain.GetNameOk(); !ok {
			sd.Name = types.StringNull()
		} else {
			sd.Name = types.StringValue(*v)
		}

		remoteDomains = append(remoteDomains, sd)
	}

	return remoteDomains, nil
}

func updateDeleted(
	ctx context.Context,
	api helpers.API,
	resourceData *data.Resource,
	domain models.Domain,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.DeleteDomain(api.ClientCtx, resourceData.ServiceID, resourceData.ServiceVersion, domain.Name.ValueString())

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
	resourceData *data.Resource,
	domain models.Domain,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.CreateDomain(api.ClientCtx, resourceData.ServiceID, resourceData.ServiceVersion)

	if !domain.Comment.IsNull() {
		clientReq.Comment(domain.Comment.ValueString())
	}

	if !domain.Name.IsNull() {
		clientReq.Name(domain.Name.ValueString())
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
	resourceData *data.Resource,
	domain models.Domain,
	resp *resource.UpdateResponse,
) error {
	clientReq := api.Client.DomainAPI.UpdateDomain(api.ClientCtx, resourceData.ServiceID, resourceData.ServiceVersion, domain.Name.ValueString())

	if !domain.Comment.IsNull() {
		clientReq.Comment(domain.Comment.ValueString())
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

// NOTE: We have to manually track each resource in a nested set attribute.
// For domains this means computing an ID for each domain, then calculating
// whether a domain has been added, deleted or modified. If any of those
// conditions are met, then we must clone the current service version.
func inspectChanges(plan, state []models.Domain) (changed bool, added, deleted, modified []models.Domain) {
	for i := range plan {
		// NOTE: We need a pointer to the resource struct so we can set an ID.
		planDomain := &plan[i]

		// ID is a computed value so we need to regenerate it from the domain name.
		if planDomain.ID.IsUnknown() {
			digest := sha256.Sum256([]byte(planDomain.Name.ValueString()))
			planDomain.ID = types.StringValue(fmt.Sprintf("%x", digest))
		}

		var foundDomain bool
		for _, stateDomain := range state {
			if planDomain.ID.ValueString() == stateDomain.ID.ValueString() {
				foundDomain = true
				// NOTE: It's not possible for the domain's Name field to not match.
				// This is because we first check the ID field matches, and that is
				// based on a hash of the domain name. Because of this we don't bother
				// checking if planDomain.Name and stateDomain.Name are not equal.
				if !planDomain.Comment.Equal(stateDomain.Comment) {
					changed = true
					modified = append(modified, *planDomain)
				}
				break
			}
		}

		if !foundDomain {
			changed = true
			added = append(added, *planDomain)
		}
	}

	for _, stateDomain := range state {
		var foundDomain bool
		for _, planDomain := range plan {
			if planDomain.ID.ValueString() == stateDomain.ID.ValueString() {
				foundDomain = true
				break
			}
		}

		if !foundDomain {
			changed = true
			deleted = append(deleted, stateDomain)
		}
	}

	return changed, added, deleted, modified
}
