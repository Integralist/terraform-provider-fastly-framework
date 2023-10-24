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

// APIKeyEnv is the environment variable we look at for a Fastly API token.
const APIKeyEnv = "FASTLY_API_TOKEN" // #nosec G101
