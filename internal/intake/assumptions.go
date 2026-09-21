// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package intake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/zyvorai/scout/internal/model"
)

// MergeAssumptions reads the customer commercial inputs. Omitted keys stay nil
// so the report can say the figure was not provided instead of inventing it.
func MergeAssumptions(inv *model.Inventory, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read assumptions: %w", err)
	}
	var a model.Assumptions
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return fmt.Errorf("assumptions.json: %w", err)
	}
	if a.Cores != nil && *a.Cores < 0 {
		return fmt.Errorf("cores must be >= 0")
	}
	for _, pair := range []struct {
		name string
		v    *float64
	}{
		{"vmwareAnnual", a.VMwareAnnual},
		{"storageAnnual", a.StorageAnnual},
		{"supportAnnual", a.SupportAnnual},
		{"migrationCost", a.MigrationCost},
		{"linkMbps", a.LinkMbps},
		{"pricePerCoreYear", a.PricePerCoreYear},
		{"minimumAnnual", a.MinimumAnnual},
	} {
		if pair.v != nil && *pair.v < 0 {
			return fmt.Errorf("%s must be >= 0", pair.name)
		}
	}
	if a.TermYears != nil && *a.TermYears <= 0 {
		return fmt.Errorf("termYears must be >= 1")
	}
	if a.IncludedDiskGiB != nil && *a.IncludedDiskGiB < 0 {
		return fmt.Errorf("includedDiskGiB must be >= 0")
	}
	inv.Assumptions = &a
	return nil
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
