package settings

import (
	"encoding/json"
	"testing"
)

func TestIntegrationsDefaultsAndRoundTrip(t *testing.T) {
	d := Defaults()
	if d.MCP.Enabled || d.MCP.AllowManage || d.MCP.AllowLocalModels || d.MCP.AllowRemoteModels || d.MCP.AllowTrigger {
		t.Fatalf("MCP defaults should be all false: %+v", d.MCP)
	}
	if d.Trigger.Enabled {
		t.Fatalf("Trigger.Enabled should default false: %+v", d.Trigger)
	}
	if !d.Trigger.AllowURLOn() {
		t.Fatal("Trigger.AllowURLOn should default true")
	}

	off := false
	d.MCP.Enabled = true
	d.MCP.AllowLocalModels = true
	d.Trigger.Enabled = true
	d.Trigger.AllowURL = &off

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Migrate(b)
	if err != nil {
		t.Fatal(err)
	}
	if !got.MCP.Enabled || !got.MCP.AllowLocalModels || got.MCP.AllowRemoteModels {
		t.Fatalf("MCP round-trip: %+v", got.MCP)
	}
	if !got.Trigger.Enabled || got.Trigger.AllowURLOn() {
		t.Fatalf("Trigger round-trip: %+v", got.Trigger)
	}
}
