package interfaces

import (
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
)

// ServiceData represents a nested entity within a Fastly service resource model.
type ServiceData interface {
	GetType() enums.ServiceType
	GetServiceID() string
	GetServiceVersion() int32
}

// TODO: Is this needed now we have more generalised ServiceData interface?
// It's only currently referenced by HasChanges method and associated interface.
//
// ServiceModel represents a Fastly service resource model.
// e.g. models.ServiceVCLResourceMode
type ServiceModel interface {
	// GetType returns the service type (e.g. enums.VCL, enums.Compute)
	GetType() enums.ServiceType
}
