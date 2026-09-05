package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zyvorai/scout/internal/model"
)

func Load(path string) (model.Inventory, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.Inventory{}, fmt.Errorf("open inventory: %w", err)
	}
	defer f.Close()
	return Decode(f)
}

func Decode(r io.Reader) (model.Inventory, error) {
	var inv model.Inventory
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&inv); err != nil {
		return inv, fmt.Errorf("decode inventory: %w", err)
	}
	if err := Validate(inv); err != nil {
		return inv, err
	}
	return inv, nil
}

func Save(path string, inv model.Inventory) error {
	if err := Validate(inv); err != nil {
		return err
	}
	b, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	return nil
}

func Validate(inv model.Inventory) error {
	if len(inv.VMs) == 0 {
		return errors.New("inventory contains no VMs")
	}
	ids := map[string]struct{}{}
	for i, vm := range inv.VMs {
		if strings.TrimSpace(vm.ID) == "" {
			return fmt.Errorf("vm[%d] has empty id", i)
		}
		if strings.TrimSpace(vm.Name) == "" {
			return fmt.Errorf("vm[%d] has empty name", i)
		}
		if _, exists := ids[vm.ID]; exists {
			return fmt.Errorf("duplicate vm id %q", vm.ID)
		}
		ids[vm.ID] = struct{}{}
		if vm.CPUs < 0 || vm.MemoryMiB < 0 {
			return fmt.Errorf("vm %q has negative capacity", vm.Name)
		}
	}
	for _, c := range inv.Connections {
		if _, ok := ids[c.From]; !ok {
			return fmt.Errorf("connection source %q is not a VM id", c.From)
		}
		if _, ok := ids[c.To]; !ok {
			return fmt.Errorf("connection target %q is not a VM id", c.To)
		}
	}
	return nil
}
