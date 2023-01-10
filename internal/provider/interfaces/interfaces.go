package interfaces

import (
	"context"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
)

// ServiceModel represents a Fastly service resource model.
// e.g. models.ServiceVCLResourceMode
type ServiceModel interface {
	GetType() enums.ServiceType
}

// NestedModel represents a nested entity within a Fastly service resource model.
// e.g. models.Domain
type NestedModel interface {
	GetType() enums.NestedType
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
		client *fastly.APIClient,
		clientCtx context.Context,
		serviceID string,
		serviceVersion int32,
		model NestedModel,
	) error
	// Read is called when the provider must read resource values in order to update state.
	// Planned state values should be read from the ReadRequest.
	// New state values set on the ReadResponse.
	Read(context.Context, resource.ReadRequest, *resource.ReadResponse) error
	// Update is called to update the state of the resource.
	// Config, planned state, and prior state values should be read from the UpdateRequest.
	// New state values set on the UpdateResponse.
	Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) error
	// Delete is called when the provider must delete the resource.
	// Config values may be read from the DeleteRequest.
	Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) error
	// HasChanges indicates if the nested resource contains configuration changes.
	HasChanges(plan ServiceModel, state ServiceModel) bool
}
