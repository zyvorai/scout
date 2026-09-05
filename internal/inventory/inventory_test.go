package inventory

import (
	"strings"
	"testing"
)

func TestDecodeRejectsUnknownFields(t *testing.T) {
	_, err := Decode(strings.NewReader(`{"source":"x","vms":[{"id":"1","name":"vm","mystery":true}]}`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestDecodeRejectsBadConnection(t *testing.T) {
	_, err := Decode(strings.NewReader(`{"source":"x","vms":[{"id":"1","name":"vm"}],"connections":[{"from":"1","to":"2"}]}`))
	if err == nil {
		t.Fatal("expected invalid connection error")
	}
}
