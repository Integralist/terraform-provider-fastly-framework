package resources

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
	commonError := errors.New("...")

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
