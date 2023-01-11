package interfaces

import (
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"
)

// ServiceData represents a nested entity within a Fastly service resource model.
type ServiceData interface {
	GetNestedType() enums.NestedType
	GetServiceID() string
	GetServiceVersion() int32
}

// ServiceModel represents a Fastly service resource model.
// e.g. models.ServiceVCLResourceMode
type ServiceModel interface {
	GetType() enums.ServiceType
}
