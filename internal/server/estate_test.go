// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/model"
)

func TestEstateFindingsEndpoint(t *testing.T) {
	mk := func(id string) model.VM {
		return model.VM{ID: id, Name: "same", OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running", NICs: []model.NIC{{MAC: "00:50:56:00:00:09"}}}
	}
	inv := model.Inventory{Source: "t", VMs: []model.VM{mk("1"), mk("2")}}
	ts := httptest.NewServer(Server{Inventory: inv}.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/estate/findings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var groups []engine.EstateFindingGroup
	if err := json.NewDecoder(resp.Body).Decode(&groups); err != nil || len(groups) != 2 {
		t.Fatalf("%v %+v", err, groups)
	}
	if groups[0].RuleID != "duplicate-mac" || groups[0].Count != 2 {
		t.Fatalf("%+v", groups)
	}
}
