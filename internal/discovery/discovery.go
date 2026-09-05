package discovery

import (
	"context"

	"github.com/zyvorai/scout/internal/model"
)

type Discoverer interface {
	Discover(context.Context) (model.Inventory, error)
}
