// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
	"github.com/zyvorai/scout/internal/plan"
)

func TestPlanValidateEndpoint(t *testing.T) {
	vm := func(id string) model.VM {
		return model.VM{ID: id, Name: id, OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running", Disks: []model.Disk{{ID: "d", SizeGiB: 10}}}
	}
	inv := model.Inventory{VMs: []model.VM{vm("app"), vm("db")}, Connections: []model.Connection{{From: "app", To: "db", Protocol: "tcp", Port: 1}}}
	ts := httptest.NewServer(Server{Inventory: inv}.Handler())
	defer ts.Close()
	post := func(body string) (int, map[string]json.RawMessage) {
		resp, err := http.Post(ts.URL+"/api/v1/plan/validate", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		m := map[string]json.RawMessage{}
		_ = json.NewDecoder(resp.Body).Decode(&m)
		return resp.StatusCode, m
	}
	code, m := post(`{"plan":{"waves":[{"wave":1,"vms":["app"]},{"wave":2,"vms":["db"]}]}}`)
	if code != 200 {
		t.Fatal(code)
	}
	var v plan.Validation
	_ = json.Unmarshal(m["validation"], &v)
	if v.OK || v.Violations[0].Code != "dependency-order" {
		t.Fatalf("%+v", v)
	}
	code, m = post(`{"plan":{"waves":[{"wave":1,"vms":["app"]},{"wave":2,"vms":["db"]}]},"moves":[{"vm":"db","toWave":1}]}`)
	var w plan.WhatIfResult
	_ = json.Unmarshal(m["whatIf"], &w)
	if code != 200 || len(w.Resolved) != 1 || !w.After.OK {
		t.Fatalf("%d %+v", code, w)
	}
	for _, bad := range []string{`{`, `{"plan":{},"x":1}`, `{"plan":{"schema":"nope"}}`, `{"plan":{"waves":[]},"moves":[{"vm":"db","toWave":1}]}` + `x`} {
		if code, _ := post(bad); code != 400 {
			t.Errorf("%q -> %d", bad, code)
		}
	}
	resp, _ := http.Get(ts.URL + "/api/v1/plan/draft")
	var d plan.Edited
	_ = json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	if d.Schema != plan.EditedSchema || len(d.Waves) != 1 {
		t.Fatalf("%+v", d)
	}
}
