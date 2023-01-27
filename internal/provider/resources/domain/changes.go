package domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/data"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// InspectChanges checks for configuration changes and persists to data model.
func (r *Resource) InspectChanges(
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
func changes(planDomains, stateDomains map[string]models.Domain) (changed bool, added, deleted, modified map[string]models.Domain) {
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
