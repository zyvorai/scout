// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package intake

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/model"
)

func TestShippedSamples(t *testing.T) {
	inv, is, err := FromCSVFile("../../sample/inventory.csv", time.Time{})
	if err != nil || HasErrors(is) || len(inv.VMs) != 3 {
		t.Fatalf("%v %v", err, is)
	}
	if !hasRuleIn(engine.Evaluate(inv), "duplicate-mac") {
		t.Fatal("csv sample should demonstrate duplicate-mac")
	}
	inv, is, err = FromScoutJSONFile("../../sample/estate-issues.json")
	if err != nil || HasErrors(is) {
		t.Fatalf("%v %v", err, is)
	}
	a := engine.Evaluate(inv)
	for _, id := range []string{"duplicate-mac", "duplicate-uuid", "duplicate-name", "missing-datastore", "missing-network", "shared-disk-split"} {
		if !hasRuleIn(a, id) {
			t.Errorf("estate sample does not trigger %s", id)
		}
	}
}

func hasRuleIn(as []model.Assessment, id string) bool {
	for _, a := range as {
		for _, f := range a.Findings {
			if f.RuleID == id {
				return true
			}
		}
	}
	return false
}

// TestSchemaFileMatchesModel guards against the documented schema drifting
// from the Go model: every JSON member the model can emit must be declared.
func TestSchemaFileMatchesModel(t *testing.T) {
	raw, err := os.ReadFile("../../docs/scout-inventory-1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Defs       map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	check := func(name string, typ reflect.Type, declared map[string]json.RawMessage) {
		var want, got []string
		for i := 0; i < typ.NumField(); i++ {
			if tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
				want = append(want, tag)
			}
		}
		for k := range declared {
			got = append(got, k)
		}
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s: model has %v, schema declares %v", name, want, got)
		}
	}
	check("inventory", reflect.TypeOf(model.Inventory{}), schema.Properties)
	check("vm", reflect.TypeOf(model.VM{}), schema.Defs["vm"].Properties)
	check("disk", reflect.TypeOf(model.Disk{}), schema.Defs["disk"].Properties)
	check("nic", reflect.TypeOf(model.NIC{}), schema.Defs["nic"].Properties)
	check("connection", reflect.TypeOf(model.Connection{}), schema.Defs["connection"].Properties)
	check("snapshot", reflect.TypeOf(model.Snapshot{}), schema.Defs["snapshot"].Properties)
	check("partition", reflect.TypeOf(model.Partition{}), schema.Defs["partition"].Properties)
}
