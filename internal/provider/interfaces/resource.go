package interfaces

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/integralist/terraform-provider-fastly-framework/internal/helpers"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/data"
)

// Resource represents an entity that has an associated Fastly API endpoint.
type Resource interface {
	// Create is called when the provider must create a new resource.
	// Config and planned state values should be read from the CreateRequest.
	// New state values set on the CreateResponse.
	Create(
		ctx context.Context,
		req *resource.CreateRequest,
		resp *resource.CreateResponse,
		api helpers.API,
		serviceData *data.Service,
	) error
	// Read is called when the provider must read resource values in order to update state.
	// Planned state values should be read from the ReadRequest.
	// New state values set on the ReadResponse.
	Read(
		ctx context.Context,
		req *resource.ReadRequest,
		resp *resource.ReadResponse,
		api helpers.API,
		serviceData *data.Service,
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
		req *resource.UpdateRequest,
		resp *resource.UpdateResponse,
		api helpers.API,
		serviceData *data.Service,
	) error
	// HasChanges indicates if the nested resource contains configuration changes.
	HasChanges() bool
	// InspectChanges checks for configuration changes and persists to data model.
	InspectChanges(
		ctx context.Context,
		req *resource.UpdateRequest,
		resp *resource.UpdateResponse,
		api helpers.API,
		serviceData *data.Service,
	) (bool, error)
}
