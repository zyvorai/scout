// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package discovery

import (
	"context"

	"github.com/zyvorai/scout/internal/model"
)

type Discoverer interface {
	Discover(context.Context) (model.Inventory, error)
}
