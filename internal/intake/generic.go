// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package intake

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zyvorai/scout/internal/inventory"
	"github.com/zyvorai/scout/internal/model"
)

// Limits for the generic importers. Reading stops at the byte cap, so a
// hostile file cannot make Scout allocate without bound.
const (
	MaxGenericBytes = 64 << 20
	MaxCSVRows      = inventory.MaxVMs
	MaxCSVColumns   = 64
	MaxCSVField     = 4096
	MaxIssues       = 100
	listSep         = ";"
)

// Issue is one problem found while reading a generic import file. Line is
// 1-based; 0 means the problem is not tied to a line.
type Issue struct {
	Line     int    `json:"line"`
	Severity string `json:"severity"` // "error" or "warning"
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

func (i Issue) String() string {
	loc := ""
	if i.Line > 0 {
		loc = fmt.Sprintf("line %d: ", i.Line)
	}
	p := ""
	if i.Path != "" {
		p = i.Path + ": "
	}
	return fmt.Sprintf("%s[%s] %s%s", loc, i.Severity, p, i.Message)
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

type issueList struct{ items []Issue }

func (l *issueList) add(line int, sev, path, format string, args ...any) {
	if len(l.items) == MaxIssues {
		l.items = append(l.items, Issue{Severity: "error", Message: fmt.Sprintf("more than %d problems; stopped reporting", MaxIssues)})
	}
	if len(l.items) > MaxIssues {
		return
	}
	l.items = append(l.items, Issue{Line: line, Severity: sev, Path: path, Message: fmt.Sprintf(format, args...)})
}

func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxGenericBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxGenericBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", MaxGenericBytes)
	}
	return data, nil
}

func clipText(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// ---- JSON: scout-inventory/1 -------------------------------------------

// FromScoutJSONFile reads a scout-inventory/1 file. The inventory is only
// meaningful when no issue is an error.
func FromScoutJSONFile(path string) (model.Inventory, []Issue, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.Inventory{}, nil, err
	}
	defer f.Close()
	inv, issues := FromScoutJSON(f)
	return inv, issues, nil
}

// FromScoutJSON decodes a scout-inventory/1 document. The "schema" member is
// required. Syntax and type errors carry the line of the failure, unknown
// members the line of their first occurrence, and semantic problems the line
// on which the offending VM object starts.
func FromScoutJSON(r io.Reader) (model.Inventory, []Issue) {
	var l issueList
	data, err := readCapped(r)
	if err != nil {
		l.add(0, "error", "", "%v", err)
		return model.Inventory{}, l.items
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		l.add(0, "error", "", "file is not valid UTF-8")
		return model.Inventory{}, l.items
	}
	var inv model.Inventory
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&inv); err != nil {
		l.add(jsonErrLine(data, err), "error", "", "%s", jsonErrMessage(err))
		return model.Inventory{}, l.items
	}
	if dec.More() {
		l.add(lineAt(data, int(dec.InputOffset())), "error", "", "unexpected data after the JSON document")
		return model.Inventory{}, l.items
	}
	if inv.Schema == "" {
		l.add(1, "error", "schema", "missing \"schema\": the file must declare %q", inventory.SchemaV1)
	}
	starts := vmLines(data)
	for _, p := range inventory.Lint(inv) {
		line := 0
		if p.VM >= 0 && p.VM < len(starts) {
			line = starts[p.VM]
		}
		l.add(line, p.Severity, p.Path, "%s", p.Message)
	}
	return inv, l.items
}

func lineAt(data []byte, off int) int {
	if off > len(data) {
		off = len(data)
	}
	if off < 0 {
		off = 0
	}
	return 1 + bytes.Count(data[:off], []byte("\n"))
}

var unknownField = regexp.MustCompile(`unknown field "([^"]{1,200})"`)

func jsonErrLine(data []byte, err error) int {
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		return lineAt(data, int(se.Offset))
	case errors.As(err, &te):
		return lineAt(data, int(te.Offset))
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return lineAt(data, len(data))
	}
	if m := unknownField.FindStringSubmatch(err.Error()); m != nil {
		if i := bytes.Index(data, []byte(`"`+m[1]+`"`)); i >= 0 {
			return lineAt(data, i)
		}
	}
	return 0
}

func jsonErrMessage(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Sprintf("field %q must be %s, found %s", te.Field, te.Type, te.Value)
	}
	return strings.TrimPrefix(err.Error(), "json: ")
}

// vmLines returns, for each element of the top-level "vms" array, the line on
// which that element starts. It returns nil if the structure cannot be walked.
func vmLines(data []byte) []int {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil
		}
		if key, _ := k.(string); key == "vms" {
			if t, err := dec.Token(); err != nil || t != json.Delim('[') {
				return nil
			}
			var lines []int
			for dec.More() {
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					return lines
				}
				lines = append(lines, lineAt(data, int(dec.InputOffset())-len(raw)))
			}
			return lines
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
	}
	return nil
}

// ---- CSV ---------------------------------------------------------------

// CSVColumns is the documented header contract, in documentation order.
// Header names are case-insensitive. Unknown columns are errors unless they
// start with "x_", which are ignored.
var CSVColumns = []string{
	"name", "id", "uuid", "os", "cpus", "memory_mib", "power_state", "firmware", "tools_status",
	"hw_version", "cbt_enabled", "secure_boot", "tpm", "encrypted", "snapshots",
	"gpu", "sriov", "usb_passthrough", "pci_passthrough", "shared_disks", "rdm",
	"disk_gib", "disk_datastores", "disk_backings",
	"networks", "macs", "ips",
}

func parseBool(s string) (val bool, set bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return false, false, true
	case "true", "t", "yes", "y", "1":
		return true, true, true
	case "false", "f", "no", "n", "0":
		return false, true, true
	}
	return false, false, false
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, listSep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// FromCSVFile reads a CSV inventory from a file.
func FromCSVFile(path string, now time.Time) (model.Inventory, []Issue, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.Inventory{}, nil, err
	}
	defer f.Close()
	inv, issues := FromCSV(f, now)
	return inv, issues, nil
}

// FromCSV reads one VM per row. See docs/IMPORT.md for the header contract.
// Every problem is reported with its line number; reading continues after a
// bad row so one run lists them all (up to MaxIssues).
func FromCSV(r io.Reader, now time.Time) (model.Inventory, []Issue) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var l issueList
	data, err := readCapped(r)
	if err != nil {
		l.add(0, "error", "", "%v", err)
		return model.Inventory{}, l.items
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		l.add(0, "error", "", "file is not valid UTF-8")
		return model.Inventory{}, l.items
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			l.add(1, "error", "", "file is empty; the first line must be the header")
		} else {
			l.add(csvLine(err, 1), "error", "", "%v", csvMsg(err))
		}
		return model.Inventory{}, l.items
	}
	if len(header) > MaxCSVColumns {
		l.add(1, "error", "", "%d columns; the limit is %d", len(header), MaxCSVColumns)
		return model.Inventory{}, l.items
	}
	known := map[string]bool{}
	for _, c := range CSVColumns {
		known[c] = true
	}
	col := map[string]int{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		switch {
		case h == "":
			l.add(1, "error", "", "column %d has an empty header", i+1)
		case strings.HasPrefix(h, "x_"):
		case !known[h]:
			l.add(1, "error", clipText(h), "unknown column; known columns: %s (columns starting with x_ are ignored)", strings.Join(CSVColumns, ", "))
		default:
			if _, dup := col[h]; dup {
				l.add(1, "error", h, "column appears more than once")
			}
			col[h] = i
		}
	}
	if _, ok := col["name"]; !ok {
		l.add(1, "error", "name", "required column \"name\" is missing")
	}
	if HasErrors(l.items) {
		return model.Inventory{}, l.items
	}

	inv := model.Inventory{Schema: inventory.SchemaV1, GeneratedAt: now.UTC(), Source: "csv", Environment: "imported"}
	ids := map[string]int{}
	var lineOf []int
	rows := 0
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line, _ := cr.FieldPos(0)
		if err != nil {
			l.add(csvLine(err, line), "error", "", "%v", csvMsg(err))
			var pe *csv.ParseError
			if !errors.As(err, &pe) {
				break
			}
			continue
		}
		rows++
		if rows > MaxCSVRows {
			l.add(line, "error", "", "more than %d rows", MaxCSVRows)
			break
		}
		if len(rec) != len(header) {
			l.add(line, "error", "", "row has %d fields but the header has %d", len(rec), len(header))
			continue
		}
		blank := true
		for _, f := range rec {
			if strings.TrimSpace(f) != "" {
				blank = false
			}
			if len(f) > MaxCSVField {
				l.add(line, "error", "", "a field is %d bytes; the limit is %d", len(f), MaxCSVField)
				blank = true // do not keep the oversize row
				break
			}
		}
		if blank {
			continue
		}
		get := func(name string) string {
			if i, ok := col[name]; ok {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		vm := model.VM{Name: get("name"), ID: get("id"), UUID: get("uuid"), OS: get("os"), PowerState: get("power_state"),
			Firmware: strings.ToLower(get("firmware")), ToolsStatus: get("tools_status")}
		if vm.ID == "" {
			vm.ID = vm.Name
		}
		if first, dup := ids[vm.ID]; dup {
			l.add(line, "error", "id", "duplicate id %q (first on line %d); add an id column to tell VMs with the same name apart", clipText(vm.ID), first)
		} else if vm.ID != "" {
			ids[vm.ID] = line
		}
		num := func(name string, max int64) int64 {
			s := get(name)
			if s == "" {
				return 0
			}
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil || n < 0 || n > max {
				l.add(line, "error", name, "%q is not an integer between 0 and %d", clipText(s), max)
				return 0
			}
			return n
		}
		vm.CPUs = int(num("cpus", 100000))
		vm.MemoryMiB = num("memory_mib", 1<<40)
		vm.Snapshots = int(num("snapshots", 100000))
		vm.HWVersion = int(num("hw_version", 1000))
		flag := func(name string, dst *bool) {
			v, _, ok := parseBool(get(name))
			if !ok {
				l.add(line, "error", name, "%q is not a boolean (true/false/yes/no/1/0)", clipText(get(name)))
			}
			*dst = v
		}
		flag("secure_boot", &vm.SecureBoot)
		flag("tpm", &vm.TPM)
		flag("encrypted", &vm.Encrypted)
		flag("gpu", &vm.GPU)
		flag("sriov", &vm.SRIOV)
		flag("usb_passthrough", &vm.USBPassthrough)
		flag("pci_passthrough", &vm.PCIPassthrough)
		flag("shared_disks", &vm.SharedDisks)
		flag("rdm", &vm.RDM)
		if v, set, ok := parseBool(get("cbt_enabled")); !ok {
			l.add(line, "error", "cbt_enabled", "%q is not a boolean (true/false/yes/no/1/0)", clipText(get("cbt_enabled")))
		} else if set {
			vm.CBTEnabled = &v
		}

		sizes, stores, backs := splitList(get("disk_gib")), splitList(get("disk_datastores")), splitList(get("disk_backings"))
		if n := len(sizes); n > 0 || len(stores) > 0 || len(backs) > 0 {
			switch {
			case n == 0:
				l.add(line, "error", "disk_gib", "disk_datastores/disk_backings need disk_gib to say how many disks there are")
			case (len(stores) > 0 && len(stores) != n) || (len(backs) > 0 && len(backs) != n):
				l.add(line, "error", "disk_gib", "disk_gib, disk_datastores and disk_backings must list the same number of disks (%d, %d, %d)", n, len(stores), len(backs))
			case n > inventory.MaxDisksPerVM:
				l.add(line, "error", "disk_gib", "%d disks; the limit is %d", n, inventory.MaxDisksPerVM)
			default:
				for i, s := range sizes {
					g, err := strconv.ParseInt(s, 10, 64)
					if err != nil || g < 0 || g > 1<<40 {
						l.add(line, "error", "disk_gib", "disk %d: %q is not a size in GiB", i+1, clipText(s))
						continue
					}
					d := model.Disk{ID: fmt.Sprintf("disk-%d", i+1), SizeGiB: g, Format: "vmdk"}
					if len(stores) > 0 {
						d.Datastore = stores[i]
					}
					if len(backs) > 0 {
						d.Backing = backs[i]
					}
					vm.Disks = append(vm.Disks, d)
				}
			}
		}
		nets, macs, ips := splitList(get("networks")), splitList(get("macs")), splitList(get("ips"))
		n := len(nets)
		if len(macs) > n {
			n = len(macs)
		}
		if len(ips) > n {
			n = len(ips)
		}
		if n > 0 {
			switch {
			case (len(nets) > 0 && len(nets) != n) || (len(macs) > 0 && len(macs) != n) || (len(ips) > 0 && len(ips) != n):
				l.add(line, "error", "networks", "networks, macs and ips must list the same number of NICs (%d, %d, %d)", len(nets), len(macs), len(ips))
			case n > inventory.MaxNICsPerVM:
				l.add(line, "error", "networks", "%d NICs; the limit is %d", n, inventory.MaxNICsPerVM)
			default:
				for i := 0; i < n; i++ {
					nic := model.NIC{ID: fmt.Sprintf("nic-%d", i+1)}
					if len(nets) > 0 {
						nic.Network = nets[i]
					}
					if len(macs) > 0 {
						nic.MAC = macs[i]
					}
					if len(ips) > 0 {
						nic.IP = ips[i]
					}
					vm.NICs = append(vm.NICs, nic)
				}
			}
		}
		inv.VMs = append(inv.VMs, vm)
		lineOf = append(lineOf, line)
	}
	// Lint checks the semantics (empty names, MAC and IP format, string
	// lengths) in one place; its VM index maps back to a CSV line.
	for _, p := range inventory.Lint(inv) {
		if strings.HasSuffix(p.Path, ".id") && strings.Contains(p.Message, "duplicate") {
			continue // already reported with the line of the first use
		}
		if p.VM < 0 && p.Path == "vms" && strings.Contains(p.Message, "no VMs") {
			if !HasErrors(l.items) {
				l.add(0, "error", "", "the file has a header but no VM rows")
			}
			continue
		}
		line := 0
		if p.VM >= 0 && p.VM < len(lineOf) {
			line = lineOf[p.VM]
		}
		l.add(line, p.Severity, p.Path, "%s", p.Message)
	}
	sort.SliceStable(l.items, func(i, j int) bool { return l.items[i].Line < l.items[j].Line })
	return inv, l.items
}

func csvLine(err error, fallback int) int {
	var pe *csv.ParseError
	if errors.As(err, &pe) && pe.StartLine > 0 {
		return pe.StartLine
	}
	return fallback
}

func csvMsg(err error) string {
	var pe *csv.ParseError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
