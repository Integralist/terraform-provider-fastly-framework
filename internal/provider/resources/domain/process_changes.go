package domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// InspectChanges checks for configuration changes and persists to data model.
func (r *Resource) InspectChanges(
	ctx context.Context,
	req *resource.UpdateRequest,
	_ *resource.UpdateResponse,
	_ helpers.API,
	_ *helpers.Service,
) (bool, error) {
	var planDomains map[string]*models.Domain // NOTE: Needs to mutate NamePast.
	var stateDomains map[string]models.Domain

	req.Plan.GetAttribute(ctx, path.Root("domains"), &planDomains)
	req.State.GetAttribute(ctx, path.Root("domains"), &stateDomains)

	r.Changed, r.Added, r.Deleted, r.Modified = changes(planDomains, stateDomains)

	tflog.Debug(context.Background(), "Domains", map[string]any{
		"added":    r.Added,
		"deleted":  r.Deleted,
		"modified": r.Modified,
		"changed":  r.Changed,
	})

	req.Plan.SetAttribute(ctx, path.Root("domains"), &planDomains)

	return r.Changed, nil
}

// HasChanges indicates if the nested resource contains configuration changes.
func (r *Resource) HasChanges() bool {
	return r.Changed
}

// MODIFIED:
// If a plan domain ID matches a state domain ID, and a nested attribute has changed, then it's been modified.
//
// ADDED:
// If a plan domain ID doesn't exist in the state, then it's a new domain.
//
// DELETED:
// If a state domain ID doesn't exist in the plan, then it's a deleted domain.
func changes(planDomains map[string]*models.Domain, stateDomains map[string]models.Domain) (changed bool, added, deleted, modified map[string]models.Domain) {
	added = make(map[string]models.Domain)
	modified = make(map[string]models.Domain)
	deleted = make(map[string]models.Domain)

	for planDomainID, planDomainData := range planDomains {
		var foundDomain bool

		for stateDomainID, stateDomainData := range stateDomains {
			if planDomainID == stateDomainID {
				foundDomain = true
				if !planDomainData.Comment.Equal(stateDomainData.Comment) {
					modified[planDomainID] = *planDomainData
					changed = true
				}
				if !planDomainData.Name.Equal(stateDomainData.Name) {
					// NOTE: We have to track the old state name for the API request.
					// The Update API endpoint requires the old domain name be provided.
					planDomainData.NamePast = types.StringValue(stateDomainData.Name.ValueString())

					modified[planDomainID] = *planDomainData
					changed = true
				}
				break
			}
		}

		if !foundDomain {
			added[planDomainID] = *planDomainData
			changed = true
		}
	}

	for stateDomainID, stateDomainData := range stateDomains {
		var foundDomain bool
		for planDomainID := range planDomains {
			if planDomainID == stateDomainID {
				foundDomain = true
				break
			}
		}

		if !foundDomain {
			deleted[stateDomainID] = stateDomainData
			changed = true
		}
	}

	return changed, added, deleted, modified
}
