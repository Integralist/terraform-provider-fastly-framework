package domain

import (
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/interfaces"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/models"
)

// NewResource returns a new resource entity.
func NewResource() interfaces.Resource {
	return &Resource{}
}

// Resource represents a Fastly entity.
type Resource struct {
	// Added represents any new resources.
	Added map[string]models.Domain
	// Deleted represents any deleted resources.
	Deleted map[string]models.Domain
	// Modified represents any modified resources.
	Modified map[string]models.Domain
	// Changed indicates if the resource has changes.
	Changed bool
}

// NOTE: Schema defined in ../../schemas/service.go
