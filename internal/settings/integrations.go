package settings

import "time"

// MCPClient is a verified AI agent the user has allowed to connect.
type MCPClient struct {
	Key           string    `json:"key"`
	Name          string    `json:"name,omitempty"`
	App           string    `json:"app,omitempty"`
	Path          string    `json:"path,omitempty"`
	Team          string    `json:"team,omitempty"`
	Allowed       time.Time `json:"allowed,omitzero"`
	AlwaysManage  bool      `json:"alwaysManage,omitempty"`
	AlwaysModels  bool      `json:"alwaysModels,omitempty"`
	AlwaysTrigger bool      `json:"alwaysTrigger,omitempty"`
}

// MCP is the local MCP server's settings (Settings → Integrations → MCP).
// Everything defaults to off.
type MCP struct {
	Enabled           bool        `json:"enabled,omitempty"`
	Acknowledged      bool        `json:"acknowledged,omitempty"`
	AllowManage       bool        `json:"allowManage,omitempty"`
	AllowLocalModels  bool        `json:"allowLocalModels,omitempty"`
	AllowRemoteModels bool        `json:"allowRemoteModels,omitempty"`
	AllowTrigger      bool        `json:"allowTrigger,omitempty"`
	Clients           []MCPClient `json:"clients,omitempty"`
}

// Trigger is the external trigger's settings (Settings → Integrations →
// External trigger). Disabled by default; AllowURL defaults to on when
// Trigger.Enabled is turned on.
type Trigger struct {
	Enabled bool `json:"enabled,omitempty"`
	// AllowURL permits voxtmini://trigger/* URL triggers while Enabled is
	// on. Nil until the user chooses: AllowURLOn treats nil as true so
	// enabling Trigger enables URL triggers by default without a migration.
	AllowURL *bool `json:"allowURL,omitempty"`
}

// AllowURLOn reports whether voxtmini://trigger/* URL triggers are allowed
// when Trigger.Enabled is on.
func (t Trigger) AllowURLOn() bool { return t.AllowURL == nil || *t.AllowURL }
