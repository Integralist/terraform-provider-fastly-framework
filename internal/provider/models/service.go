package models

import "github.com/integralist/terraform-provider-fastly-framework/internal/provider/enums"

// Service is a wrapper to ensure nested entities implement
// interfaces.Service (consumed by interfaces.Resource methods).
type Service struct {
	// ServiceID is the ID for the Fastly service.
	ServiceID string
	// ServiceVersion is the current version for the Fastly service.
	ServiceVersion int32
	// State is the complete Terraform state data the nested model can reference.
	State any
	// Type is the nested model type (e.g. enums.Domain)
	Type enums.NestedType
}

func (d Service) GetNestedType() enums.NestedType {
	return d.Type
}

func (d Service) GetServiceID() string {
	return d.ServiceID
}

func (d Service) GetServiceVersion() int32 {
	return d.ServiceVersion
}
