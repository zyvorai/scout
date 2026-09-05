package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVMwareDiscovery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			u, p, ok := r.BasicAuth()
			if !ok || u != "admin" || p != "secret" {
				http.Error(w, "unauthorized", 401)
				return
			}
			fmt.Fprint(w, `"session-1"`)
		case http.MethodDelete:
			w.WriteHeader(204)
		default:
			http.Error(w, "bad method", 405)
		}
	})
	mux.HandleFunc("/api/vcenter/vm", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("vmware-api-session-id") != "session-1" {
			http.Error(w, "unauthorized", 401)
			return
		}
		fmt.Fprint(w, `[{"vm":"vm-1","name":"prod-1","power_state":"POWERED_ON","cpu_count":4,"memory_size_MiB":8192}]`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	inv, err := (VMware{BaseURL: ts.URL, Username: "admin", Password: "secret", Client: ts.Client()}).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.VMs) != 1 || inv.VMs[0].Name != "prod-1" || inv.VMs[0].MemoryMiB != 8192 {
		t.Fatalf("inventory=%+v", inv)
	}
}
