// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package discovery

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zyvorai/scout/internal/model"
)

type VMware struct {
	BaseURL  string
	Username string
	Password string
	Insecure bool
	Client   *http.Client
}

type vmwareVM struct {
	VM            string `json:"vm"`
	Name          string `json:"name"`
	PowerState    string `json:"power_state"`
	CPUCount      int    `json:"cpu_count"`
	MemorySizeMiB int64  `json:"memory_size_MiB"`
}

func (v VMware) Discover(ctx context.Context) (model.Inventory, error) {
	base, err := url.Parse(strings.TrimRight(v.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return model.Inventory{}, fmt.Errorf("invalid vCenter URL")
	}
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: v.Insecure}}} //nolint:gosec -- explicit opt-in flag
	}

	session, err := v.createSession(ctx, client, base.String())
	if err != nil {
		return model.Inventory{}, err
	}
	defer v.deleteSession(context.Background(), client, base.String(), session)

	var list []vmwareVM
	if err := v.getJSON(ctx, client, base.String()+"/api/vcenter/vm", session, &list); err != nil {
		return model.Inventory{}, fmt.Errorf("list vCenter VMs: %w", err)
	}
	inv := model.Inventory{GeneratedAt: time.Now().UTC(), Source: "vmware-vcenter", Environment: base.Host, Metadata: map[string]string{"vcenter": base.Host}}
	for _, item := range list {
		inv.VMs = append(inv.VMs, model.VM{ID: item.VM, Name: item.Name, CPUs: item.CPUCount, MemoryMiB: item.MemorySizeMiB, PowerState: item.PowerState, Firmware: "unknown", ToolsStatus: "unknown", Tags: map[string]string{"provider": "vmware"}})
	}
	if len(inv.VMs) == 0 {
		return inv, fmt.Errorf("vCenter returned no VMs")
	}
	return inv, nil
}

func (v VMware) createSession(ctx context.Context, client *http.Client, base string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/session", bytes.NewReader(nil))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(v.Username, v.Password)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vCenter login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("vCenter login returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var token string
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", fmt.Errorf("decode vCenter session: %w", err)
	}
	return token, nil
}

func (v VMware) deleteSession(ctx context.Context, client *http.Client, base, session string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/session", nil)
	if err != nil {
		return
	}
	req.Header.Set("vmware-api-session-id", session)
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func (v VMware) getJSON(ctx context.Context, client *http.Client, endpoint, session string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("vmware-api-session-id", session)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
