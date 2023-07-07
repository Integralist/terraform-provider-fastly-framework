// Copied from https://github.com/hashicorp/terraform-plugin-framework/blob/main/website/docs/plugin/framework/resources/plan-modification.mdx#creating-attribute-plan-modifiers
//
// EXAMPLE:
//
//  PlanModifiers: []planmodifier.String{
// 	  helpers.StringDefaultModifier{Default: "Managed by Terraform"},
//  },

package helpers

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// StringDefaultModifier is a plan modifier that sets a default value for a
// types.StringType attribute when it is not configured. The attribute must be
// marked as Optional and Computed.
type StringDefaultModifier struct {
	Default string
}

// Description returns a plain text description of the validator's behavior, suitable for a practitioner to understand its impact.
func (m StringDefaultModifier) Description(_ context.Context) string {
	return fmt.Sprintf("If value is not configured, defaults to %s", m.Default)
}

// MarkdownDescription returns a markdown formatted description of the validator's behavior, suitable for a practitioner to understand its impact.
func (m StringDefaultModifier) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("If value is not configured, defaults to `%s`", m.Default)
}

// PlanModifyString runs the logic of the plan modifier.
// Access to the configuration, plan, and state is available in `req`, while
// `resp` contains fields for updating the planned value, triggering resource
// replacement, and returning diagnostics.
func (m StringDefaultModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// If the value is known, do not set default value.
	//
	// WARNING: There might be issues with this implementation.
	// https://github.com/hashicorp/terraform-plugin-framework/issues/596
	if !req.PlanValue.IsUnknown() {
		return
	}

	resp.PlanValue = types.StringValue(m.Default)
}
