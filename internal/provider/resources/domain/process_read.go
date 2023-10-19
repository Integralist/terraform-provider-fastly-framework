package domain

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/data"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// Read is called when the provider must read resource values in order to update state.
// Planned state values should be read from the ReadRequest.
// New state values set on the ReadResponse.
func (r *Resource) Read(
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
		remoteDomainData := models.Domain{
			Name: types.StringValue(remoteDomainName),
		}

		// NOTE: The API has no concept of an ID for a domain.
		// The ID is arbitrarily chosen by the user and set in their config.
		// The ID must be unique and is used as a key for accessing a domain.
		var (
			found          bool
			remoteDomainID string
		)

		for stateDomainID, stateDomainData := range stateDomains {
			if stateDomainData.Name.ValueString() == remoteDomainName {
				remoteDomainID = stateDomainID
				found = true
			}
		}

		// If we can't match a remote domain with anything in the state,
		// then we'll give the domain a uuid and treat it as a domain added
		// out-of-band from Terraform.
		if !found {
			remoteDomainID = uuid.New().String()
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
		if v, ok := remoteDomain.GetCommentOk(); ok {
			// Set comment to whatever is returned by the API (could be an empty
			// string and that might be because the user set that explicitly or it
			// could be because it was never set and the API is just returning an
			// empty string as a default value).
			remoteDomainData.Comment = types.StringValue(*v)

			// We need to check if the user config has set the comment.
			// If not, then we'll again set the value to null to avoid a plan diff.
			// See the above WARNING for the details.
			for _, stateDomainData := range stateDomains {
				if stateDomainData.Name.ValueString() == remoteDomainName {
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

		remoteDomains[remoteDomainID] = remoteDomainData
	}

	return remoteDomains, nil
}
