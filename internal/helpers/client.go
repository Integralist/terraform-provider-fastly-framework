package helpers

import (
	"context"

	"github.com/fastly/fastly-go/fastly"
)

// API is a simple helper for avoiding passing large service model data structure.
type API struct {
	Client    *fastly.APIClient
	ClientCtx context.Context
}
