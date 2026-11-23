// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/diff"
	"github.com/zyvorai/scout/internal/model"
)

func TestDiffEndpoint(t *testing.T) {
	cur := model.Inventory{VMs: []model.VM{{ID: "1", Name: "a", OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running", RDM: true}}}
	ts := httptest.NewServer(Server{Inventory: cur}.Handler())
	defer ts.Close()
	oldDoc := `{"vms":[{"id":"1","name":"a","os":"RHEL 9","firmware":"uefi","toolsStatus":"running"}]}`
	post := func(query, body string) (int, struct {
		Result diff.Result `json:"result"`
		Gate   diff.Gate   `json:"gate"`
	}) {
		var out struct {
			Result diff.Result `json:"result"`
			Gate   diff.Gate   `json:"gate"`
		}
		resp, err := http.Post(ts.URL+"/api/v1/diff"+query, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	code, out := post("?failOn=regression", oldDoc)
	if code != 200 || out.Result.Regressed != 1 || !out.Gate.Failed {
		t.Fatalf("%d %+v", code, out)
	}
	if code, out = post("", oldDoc); code != 200 || out.Gate.Failed {
		t.Fatalf("%d %+v", code, out)
	}
	for _, bad := range []struct{ q, body string }{{"", `{}`}, {"", `nope`}, {"?failOn=zzz", oldDoc}, {"", `{"assessments":[{"vmId":"1","status":"x"}]}`}} {
		if code, _ := post(bad.q, bad.body); code != 400 {
			t.Errorf("%q%q -> %d", bad.q, bad.body, code)
		}
	}
	if code, _ := post("", strings.Repeat(" ", 9<<20)); code != 400 {
		t.Errorf("oversize -> %d", code)
	}
}
