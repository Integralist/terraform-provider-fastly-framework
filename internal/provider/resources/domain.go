package resources

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/interfaces"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

func DomainCreate(
	ctx context.Context,
	r *ServiceVCLResource,
	domains []models.Domain,
	id string,
	version int32,
	resp *resource.CreateResponse,
) error {
	// TODO: Consider mapstructure for abstracting at a more generic level.
	// https://pkg.go.dev/github.com/mitchellh/mapstructure might help for update diffing.

	// TODO: Define actual error message.
	commonError := errors.New("todo")

	for i := range domains {
		domain := &domains[i]

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
		clientReq := r.client.DomainAPI.CreateDomain(r.clientCtx, id, version)

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
			return commonError
		}
		if httpResp.StatusCode != http.StatusOK {
			tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
			return commonError
		}
	}

	return nil
}

func DomainRead(
	ctx context.Context,
	r *ServiceVCLResource,
	clientResp *fastly.ServiceDetail,
	state *models.ServiceVCLResourceModel,
	resp *resource.ReadResponse,
) error {
	clientDomainReq := r.client.DomainAPI.ListDomains(r.clientCtx, clientResp.GetID(), int32(state.Version.ValueInt64()))

	// TODO: After abstraction rename back to clientResp.
	clientDomainResp, httpResp, err := clientDomainReq.Execute()
	if err != nil {
		tflog.Trace(ctx, "Fastly DomainAPI.ListDomains error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list domains, got error: %s", err))
		return err
	}
	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Fastly API error", map[string]any{"http_resp": httpResp})
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unsuccessful status code: %s", httpResp.Status))
		return err
	}

	var domains []models.Domain

	// TODO: Rename domainData to domain once moved to separate package.
	for _, domainData := range clientDomainResp {
		domainName := domainData.GetName()
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
		if v, ok := domainData.GetCommentOk(); ok {
			// Set comment to whatever is returned by the API (could be an empty
			// string and that might be because the user set that explicitly or it
			// could be because it was never set and the API is just returning an
			// empty string as a default value).
			sd.Comment = types.StringValue(*v)

			// We need to check if the user config has set the comment.
			// If not, then we'll again set the value to null to avoid a plan diff.
			// See the above WARNING for the details.
			for _, stateDomain := range state.Domains {
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
			if len(state.Domains) == 0 && *v == "" {
				sd.Comment = types.StringNull()
			}
		} else {
			sd.Comment = types.StringNull()
		}

		if v, ok := domainData.GetNameOk(); !ok {
			sd.Name = types.StringNull()
		} else {
			sd.Name = types.StringValue(*v)
		}

		domains = append(domains, sd)
	}

	state.Domains = domains

	return nil
}

func testing(service interfaces.Service) {
	if service.GetType() == enums.VCL {
		if v, ok := service.(*models.ServiceVCLResourceModel); ok {
			fmt.Printf("service converted: %T %+v\n", v, v)
		} else {
			fmt.Printf("service data: %T %+v\n", service, service)
		}
	}
}

// FIXME: We need an abstraction like SetDiff from the original provider.
// We compare the plan set to the state set and determine what changed.
// e.g. 'added', 'modified', 'deleted' and calls relevant API.
// Needs a 'key' for each resource (sometimes 'name' but has to be unique).
// Unless we switch to MapNestedAttribute which provides a key by design.
//
// If plan 'key' exists in prior state, then it's modified.
// Otherwise resource is new.
// If state 'key' doesn't exist in plan, then it's deleted.
//
// If domain hashed is found in state, then it already exists and might be modified.
// If domain hashed is not found in state, then it is either new or an existing domain that was renamed.
// We then separately loop the state and see if it exists in the plan (if it doesn't, then it's deleted)
func DomainChanges(
	plan *models.ServiceVCLResourceModel,
	state *models.ServiceVCLResourceModel,
) (shouldClone bool, added, deleted, modified []models.Domain) {
	testing(plan)
	// NOTE: We have to manually track each resource in a nested set attribute.
	// For domains this means computing an ID for each domain, then calculating
	// whether a domain has been added, deleted or modified. If any of those
	// conditions are met, then we must clone the current service version.
	// TODO: Abstract domain and other resources
	for i := range plan.Domains {
		// NOTE: We need a pointer to the resource struct so we can set an ID.
		planDomain := &plan.Domains[i]

		// ID is a computed value so we need to regenerate it from the domain name.
		if planDomain.ID.IsUnknown() {
			digest := sha256.Sum256([]byte(planDomain.Name.ValueString()))
			planDomain.ID = types.StringValue(fmt.Sprintf("%x", digest))
		}

		var foundDomain bool
		for _, stateDomain := range state.Domains {
			if planDomain.ID.ValueString() == stateDomain.ID.ValueString() {
				foundDomain = true
				// NOTE: It's not possible for the domain's Name field to not match.
				// This is because we first check the ID field matches, and that is
				// based on a hash of the domain name. Because of this we don't bother
				// checking if planDomain.Name and stateDomain.Name are not equal.
				if !planDomain.Comment.Equal(stateDomain.Comment) {
					shouldClone = true
					modified = append(modified, *planDomain)
				}
				break
			}
		}

		if !foundDomain {
			shouldClone = true
			added = append(added, *planDomain)
		}
	}

	for _, stateDomain := range state.Domains {
		var foundDomain bool
		for _, planDomain := range plan.Domains {
			if planDomain.ID.ValueString() == stateDomain.ID.ValueString() {
				foundDomain = true
				break
			}
		}

		if !foundDomain {
			shouldClone = true
			deleted = append(deleted, stateDomain)
		}
	}

	return shouldClone, added, deleted, modified
}

func DomainUpdate(
	ctx context.Context,
	r *ServiceVCLResource,
	added, deleted, modified []models.Domain,
	plan *models.ServiceVCLResourceModel,
	resp *resource.UpdateResponse,
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

	for _, domain := range deleted {
		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.DeleteDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()), domain.Name.ValueString())

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
	}

	for _, domain := range added {
		// TODO: Abstract the following API call into a function as it's called multiple times.

		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.CreateDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()))

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
	}

	for _, domain := range modified {
		// TODO: Check if the version we have is correct.
		// e.g. should it be latest 'active' or just latest version?
		// It should depend on `activate` field but also whether the service pre-exists.
		// The service might exist if it was imported or a secondary config run.
		clientReq := r.client.DomainAPI.UpdateDomain(r.clientCtx, plan.ID.ValueString(), int32(plan.Version.ValueInt64()), domain.Name.ValueString())

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
	}

	return nil
}
