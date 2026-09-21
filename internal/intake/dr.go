// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package intake

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zyvorai/scout/internal/model"
)

// MergeDR reads the version-1 dr.yaml subset:
//
//	lastSuccessfulBackup: 2026-09-01T00:00:00Z
//	targetRPOMinutes: 15
//	lastTestRestore: 2026-06-01T00:00:00Z
//	offsiteCopy: true
//
// Nested YAML and vendor backup binaries are rejected. This is a declaration
// the customer writes, not a parsed Veeam or Rubrik database.
func MergeDR(inv *model.Inventory, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read dr file: %w", err)
	}
	defer f.Close()
	dr := &model.DRPosture{}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-") || strings.Contains(line, "{") || strings.Contains(line, "[") {
			return fmt.Errorf("dr.yaml line %d: only flat key: value lines are accepted", lineNo)
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return fmt.Errorf("dr.yaml line %d: expected key: value", lineNo)
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		switch key {
		case "lastSuccessfulBackup":
			t, err := parseTimeStrict(val)
			if err != nil {
				return fmt.Errorf("dr.yaml lastSuccessfulBackup: %w", err)
			}
			dr.LastSuccessfulBackup = &t
		case "targetRPOMinutes":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return fmt.Errorf("dr.yaml targetRPOMinutes must be a non-negative integer")
			}
			dr.TargetRPOMinutes = &n
		case "lastTestRestore":
			t, err := parseTimeStrict(val)
			if err != nil {
				return fmt.Errorf("dr.yaml lastTestRestore: %w", err)
			}
			dr.LastTestRestore = &t
		case "offsiteCopy":
			switch strings.ToLower(val) {
			case "true", "yes":
				v := true
				dr.OffsiteCopy = &v
			case "false", "no":
				v := false
				dr.OffsiteCopy = &v
			default:
				return fmt.Errorf("dr.yaml offsiteCopy must be true or false")
			}
		default:
			return fmt.Errorf("dr.yaml line %d: unknown key %q", lineNo, key)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	inv.DR = dr
	return nil
}

func parseTimeStrict(val string) (time.Time, error) {
	t := parseTime(val)
	if t.IsZero() {
		return time.Time{}, fmt.Errorf("unrecognized time %q", val)
	}
	return t, nil
}
