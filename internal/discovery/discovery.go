// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-1.0

package discovery

import (
	"context"

	"github.com/zyvorai/scout/internal/model"
)

type Discoverer interface {
	Discover(context.Context) (model.Inventory, error)
}
