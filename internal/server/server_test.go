// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/scout/internal/model"
	"github.com/zyvorai/scout/internal/plan"
)

func TestAPI(t *testing.T) {
	inv := model.Inventory{Source: "test", Environment: "lab", VMs: []model.VM{{ID: "1", Name: "vm1", Firmware: "uefi", ToolsStatus: "running", CPUs: 2, MemoryMiB: 2048}}}
	ts := httptest.NewServer(Server{Inventory: inv}.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/summary")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var s model.Summary
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.TotalVMs != 1 || s.Ready != 1 {
		t.Fatalf("summary=%+v", s)
	}
	resp2, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
}

func TestExportAndPlanEndpoints(t *testing.T) {
	inv := model.Inventory{Source: "test", Environment: "lab", VMs: []model.VM{
		{ID: "1", Name: "vm1", OS: "Oracle Solaris 11", Firmware: "uefi", ToolsStatus: "running", Disks: []model.Disk{{ID: "d", SizeGiB: 100}}},
		{ID: "2", Name: "vm2", OS: "RHEL 9", Firmware: "uefi", ToolsStatus: "running", Disks: []model.Disk{{ID: "d", SizeGiB: 100}}},
	}}
	ts := httptest.NewServer(Server{Inventory: inv}.Handler())
	defer ts.Close()

	get := func(path string) (*http.Response, string) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var sb strings.Builder
		_, _ = io.Copy(&sb, resp.Body)
		return resp, sb.String()
	}
	resp, body := get("/api/v1/sarif")
	if resp.StatusCode != 200 || !strings.Contains(body, `"guest-os-support"`) || resp.Header.Get("Content-Type") != "application/sarif+json" {
		t.Fatalf("sarif %d %s", resp.StatusCode, body)
	}
	resp, body = get("/api/v1/findings.csv")
	if resp.StatusCode != 200 || !strings.HasPrefix(body, "vmId,vm,wave") || !strings.Contains(body, "guest-os-support") {
		t.Fatalf("csv %d %s", resp.StatusCode, body)
	}
	resp, body = get("/api/v1/plan?linkMbps=1000&maxConcurrent=1")
	var p plan.Plan
	if err := json.Unmarshal([]byte(body), &p); err != nil || resp.StatusCode != 200 {
		t.Fatalf("plan %d %v %s", resp.StatusCode, err, body)
	}
	if p.Totals.VMs != 1 || p.Totals.Skipped != 1 || p.Constraints.MaxConcurrent != 1 {
		t.Fatalf("plan %+v", p)
	}
	if resp, _ = get("/api/v1/plan?maxConcurrent=x"); resp.StatusCode != 400 {
		t.Fatalf("bad query status %d", resp.StatusCode)
	}
	if resp, _ = get("/api/v1/plan?efficiency=2"); resp.StatusCode != 400 {
		t.Fatalf("bad efficiency status %d", resp.StatusCode)
	}
	post, err := http.Post(ts.URL+"/api/v1/plan", "application/json", strings.NewReader(`{"linkMbps":500,"windowMinutes":60}`))
	if err != nil {
		t.Fatal(err)
	}
	post.Body.Close()
	if post.StatusCode != 200 {
		t.Fatalf("post status %d", post.StatusCode)
	}
	bad, _ := http.Post(ts.URL+"/api/v1/plan", "application/json", strings.NewReader(`{"bogus":1}`))
	bad.Body.Close()
	if bad.StatusCode != 400 {
		t.Fatalf("unknown field status %d", bad.StatusCode)
	}
}
