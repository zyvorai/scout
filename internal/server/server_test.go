package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zyvorai/scout/internal/model"
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
