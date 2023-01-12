package interfaces

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
)

// ResourceData represents the top-level resource and its associated data.
type ResourceData interface {
	GetPlan() any
	GetServiceID() string
	GetServiceVersion() int32
	GetState() any
	GetType() enums.ServiceType
	SetPlan(plan any)
	SetState(state any)
}

// Resource represents an entity that has an associated Fastly API endpoint.
type Resource interface {
	// Create is called when the provider must create a new resource.
	// Config and planned state values should be read from the CreateRequest.
	// New state values set on the CreateResponse.
	Create(
		ctx context.Context,
		req resource.CreateRequest,
		resp *resource.CreateResponse,
		api helpers.API,
		resourceData ResourceData,
	) error
	// Read is called when the provider must read resource values in order to update state.
	// Planned state values should be read from the ReadRequest.
	// New state values set on the ReadResponse.
	Read(
		ctx context.Context,
		req resource.ReadRequest,
		resp *resource.ReadResponse,
		api helpers.API,
		resourceData ResourceData,
	) error
	// Update is called to update the state of the resource.
	// Config, planned state, and prior state values should be read from the UpdateRequest.
	// New state values set on the UpdateResponse.
	//
	// NOTE: The CRUD boundaries are blurred due to Fastly's API model.
	// The update operation doesn't just handle updates.
	// It must also handle future additions/deletions.
	// This is because the parent service only calls 'Create' once.
	// All other modifications to the service resource will come to 'Update'.
	Update(
		ctx context.Context,
		req resource.UpdateRequest,
		resp *resource.UpdateResponse,
		api helpers.API,
		resourceData ResourceData,
	) error
	// GetType returns the nested resource type (e.g. enums.Domain)
	GetType() enums.NestedType
	// HasChanges indicates if the nested resource contains configuration changes.
	HasChanges() bool
	// InspectChanges checks for configuration changes and persists to data model.
	InspectChanges(resourceData ResourceData) (bool, error)
}
