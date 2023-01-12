package helpers

import (
	"context"

	"github.com/fastly/fastly-go/fastly"
)

type API struct {
	Client    *fastly.APIClient
	ClientCtx context.Context
}
