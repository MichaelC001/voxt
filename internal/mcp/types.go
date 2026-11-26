package mcp

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by Host implementations and mapped to MCP
// tool error codes.
var (
	ErrOff       = errors.New("MCP server is turned off in VoxtMini Settings")
	ErrNotFound  = errors.New("not found")
	ErrBusy      = errors.New("a recording session is already in progress")
	ErrRemoteOff = errors.New("remote models are not allowed for MCP in VoxtMini Settings")
)

// Policy is the dynamic permission configuration fetched from VoxtMini.
type Policy struct {
	Enabled           bool
	AllowManage       bool
	AllowLocalModels  bool
	AllowRemoteModels bool
	AllowTrigger      bool
	// Clients maps a verified client key to its persisted grants.
	Clients map[string]ClientGrant
}

// ClientGrant records what the user has permanently allowed for a client.
type ClientGrant struct {
	Allowed       bool
	AlwaysManage  bool
	AlwaysModels  bool
	AlwaysTrigger bool
}

// Client looks up key's grant in p.
func (p Policy) Client(key string) (ClientGrant, bool) {
	if p.Clients == nil {
		return ClientGrant{}, false
	}
	g, ok := p.Clients[key]
	return g, ok
}

// Prompter asks the user in VoxtMini to approve an agent or action.
type Prompter interface {
	Ask(ctx context.Context, q Question) Answer
}

// Host is the narrow interface between the MCP server and VoxtMini.
// All methods may be called from server goroutines.
type Host interface {
	// Policy returns the current MCP permissions.
	Policy() Policy
	// Ready returns nil when VoxtMini can serve MCP calls.
	Ready() error
	// Changed notifies the UI that connected clients or audit state changed.
	Changed()

	// Read operations.
	Overview(ctx context.Context) (Overview, error)
	ListHistory(ctx context.Context, req HistoryQuery) (HistoryPage, error)
	GetHistory(ctx context.Context, id string) (HistoryDetail, error)
	ListTerms(ctx context.Context, search, category string) ([]Term, error)
	ListCategories(ctx context.Context) ([]Category, error)
	ListAppGroups(ctx context.Context) ([]AppGroup, error)
	ListModels(ctx context.Context, kind string) ([]ModelInfo, error)
	ListMicrophones(ctx context.Context) ([]Microphone, error)

	// Manage operations.
	UpsertTerms(ctx context.Context, terms []TermInput) ([]Term, error)
	DeleteTerm(ctx context.Context, id string) error
	UpsertCategory(ctx context.Context, in CategoryInput) (Category, error)
	UpsertAppGroup(ctx context.Context, in AppGroupInput) (AppGroup, error)
	DeleteAppGroup(ctx context.Context, id string) error
	DeleteHistory(ctx context.Context, id string) error

	// Model operations.
	TranscribeAudio(ctx context.Context, req TranscribeRequest) (TranscribeResult, error)
	TranslateText(ctx context.Context, req TranslateRequest) (TranslateResult, error)
	RewriteText(ctx context.Context, req RewriteRequest) (RewriteResult, error)
	ChatModel(ctx context.Context, req ChatRequest) (ChatResult, error)

	// Trigger operations.
	TriggerSession(ctx context.Context, req TriggerRequest) (TriggerResult, error)
	SessionStatus(ctx context.Context) (SessionStatus, error)
}

// Overview is returned by voxtmini_overview.
type Overview struct {
	AppVersion  string         `json:"appVersion"`
	Session     SessionStatus  `json:"session"`
	Permissions PolicySummary  `json:"permissions"`
	Counts      OverviewCounts `json:"counts"`
	Models      []ModelSummary `json:"models"`
	Microphones []Microphone   `json:"microphones"`
}

// PolicySummary exposes active MCP permissions to the agent.
type PolicySummary struct {
	Read         bool `json:"read"`
	Manage       bool `json:"manage"`
	LocalModels  bool `json:"localModels"`
	RemoteModels bool `json:"remoteModels"`
	Trigger      bool `json:"trigger"`
}

// OverviewCounts summarizes stored VoxtMini data.
type OverviewCounts struct {
	History    int `json:"history"`
	Terms      int `json:"terms"`
	Categories int `json:"categories"`
	AppGroups  int `json:"appGroups"`
}

// ModelSummary is a compact model description for overview.
type ModelSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`   // "speech" | "language"
	Source string `json:"source"` // "local" | "apple" | "remote"
	Ready  bool   `json:"ready"`
}

// HistoryQuery filters history entries.
type HistoryQuery struct {
	Kind   string
	Query  string
	Limit  int
	Cursor string
}

// HistoryPage is one page of history summaries.
type HistoryPage struct {
	Entries    []HistoryItem `json:"entries"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// HistoryItem is a compact history entry returned by list_history.
type HistoryItem struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	AppName   string    `json:"appName,omitempty"`
	Preview   string    `json:"preview,omitempty"`
	HasAudio  bool      `json:"hasAudio"`
}

// HistoryDetail is the full record returned by get_history.
type HistoryDetail struct {
	ID          string       `json:"id"`
	CreatedAt   time.Time    `json:"createdAt"`
	Kind        string       `json:"kind"`
	Status      string       `json:"status"`
	Error       string       `json:"error,omitempty"`
	AppName     string       `json:"appName,omitempty"`
	AppBundle   string       `json:"appBundle,omitempty"`
	URLHost     string       `json:"urlHost,omitempty"`
	RuleName    string       `json:"ruleName,omitempty"`
	RawText     string       `json:"rawText,omitempty"`
	SourceText  string       `json:"sourceText,omitempty"`
	PromptText  string       `json:"promptText,omitempty"`
	Result      string       `json:"result,omitempty"`
	TargetLang  string       `json:"targetLang,omitempty"`
	Delivery    string       `json:"delivery,omitempty"`
	ASRModel    string       `json:"asrModel,omitempty"`
	LLMModel    string       `json:"llmModel,omitempty"`
	AudioMS     int64        `json:"audioMs,omitempty"`
	ASRMS       int64        `json:"asrMs,omitempty"`
	LLMMS       int64        `json:"llmMs,omitempty"`
	TotalMS     int64        `json:"totalMs,omitempty"`
	Corrections []Correction `json:"corrections,omitempty"`
	FollowUps   []FollowUp   `json:"followUps,omitempty"`
	Audio       *AudioMeta   `json:"audio,omitempty"`
}

// Correction is a dictionary replacement applied to a history entry.
type Correction struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// FollowUp is a follow-up question and answer on a history entry.
type FollowUp struct {
	Question string    `json:"question"`
	Answer   string    `json:"answer"`
	At       time.Time `json:"at"`
}

// AudioMeta describes a stored recording without returning raw audio bytes.
type AudioMeta struct {
	HasAudio   bool   `json:"hasAudio"`
	DurationMS int64  `json:"durationMs,omitempty"`
	Path       string `json:"path,omitempty"`
}

// Term is a dictionary entry.
type Term struct {
	ID       string   `json:"id"`
	Spelling string   `json:"spelling"`
	Hints    []string `json:"hints,omitempty"`
	Category string   `json:"category,omitempty"`
	Group    string   `json:"group,omitempty"`
	Enabled  bool     `json:"enabled"`
	Uses     int      `json:"uses"`
}

// TermInput is the payload for upsert_terms.
type TermInput struct {
	ID       string   `json:"id,omitempty" jsonschema:"Existing term id to update; omit to create"`
	Spelling string   `json:"spelling" jsonschema:"Canonical spelling of the term"`
	Hints    []string `json:"hints,omitempty" jsonschema:"Spoken variants or common misspellings"`
	Category string   `json:"category,omitempty" jsonschema:"Category name or id"`
	Group    string   `json:"group,omitempty" jsonschema:"Optional app enhancement group scope"`
	Enabled  *bool    `json:"enabled,omitempty" jsonschema:"Whether the term is active (default true)"`
}

// Category is a dictionary category.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CategoryInput is the payload for upsert_category.
type CategoryInput struct {
	ID   string `json:"id,omitempty" jsonschema:"Existing category id to rename; omit to create"`
	Name string `json:"name" jsonschema:"Category name"`
}

// AppGroup is an app enhancement group.
type AppGroup struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Apps     []string `json:"apps,omitempty"`
	Domains  []string `json:"domains,omitempty"`
	Prompt   string   `json:"prompt,omitempty"`
	AfterKey string   `json:"afterKey,omitempty"`
	Features []string `json:"features,omitempty"`
}

// AppGroupInput is the payload for upsert_app_group.
type AppGroupInput struct {
	ID       string   `json:"id,omitempty" jsonschema:"Existing group id to update; omit to create"`
	Name     string   `json:"name" jsonschema:"Display name of the app group"`
	Apps     []string `json:"apps,omitempty" jsonschema:"macOS bundle IDs in this group"`
	Domains  []string `json:"domains,omitempty" jsonschema:"Website domains or URL rules"`
	Prompt   string   `json:"prompt,omitempty" jsonschema:"Custom scene prompt for this group"`
	AfterKey string   `json:"after_key,omitempty" jsonschema:"Key pressed after pasting (e.g. Return)"`
	Enabled  *bool    `json:"enabled,omitempty" jsonschema:"Whether the group is enabled (default true)"`
	Features []string `json:"features,omitempty" jsonschema:"Features: transcribe, translate, rewrite"`
}

// ModelInfo describes a configured or installed model (never includes API keys).
type ModelInfo struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`   // "speech" | "language"
	Source    string   `json:"source"` // "local" | "apple" | "remote"
	Installed bool     `json:"installed"`
	Ready     bool     `json:"ready"`
	Features  []string `json:"features,omitempty"`
	Default   bool     `json:"default,omitempty"`
}

// Microphone describes an audio input device.
type Microphone struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Transport string `json:"transport,omitempty"` // "usb" | "bluetooth" | "builtin" | "virtual"
	IsDefault bool   `json:"isDefault"`
	Priority  int    `json:"priority"`
}

// TranscribeRequest invokes VoxtMini's speech recognition pipeline.
type TranscribeRequest struct {
	AudioPath   string
	ModelID     string
	Cleanup     bool
	Language    string
	AllowRemote bool
}

// TranscribeResult is returned by transcribe_audio.
type TranscribeResult struct {
	Text      string `json:"text"`
	RawText   string `json:"rawText,omitempty"`
	Model     string `json:"model"`
	Source    string `json:"source"`
	LatencyMS int64  `json:"latencyMs"`
}

// TranslateRequest invokes VoxtMini's translation pipeline.
type TranslateRequest struct {
	Text           string
	TargetLanguage string
	ModelID        string
	AllowRemote    bool
}

// TranslateResult is returned by translate_text.
type TranslateResult struct {
	Text           string `json:"text"`
	TargetLanguage string `json:"targetLanguage"`
	Model          string `json:"model"`
	Source         string `json:"source"`
	LatencyMS      int64  `json:"latencyMs"`
}

// RewriteRequest invokes VoxtMini's rewrite pipeline.
type RewriteRequest struct {
	Instruction string
	Text        string
	ModelID     string
	AllowRemote bool
}

// RewriteResult is returned by rewrite_text.
type RewriteResult struct {
	Text      string `json:"text"`
	Model     string `json:"model"`
	Source    string `json:"source"`
	LatencyMS int64  `json:"latencyMs"`
}

// ChatRequest invokes a language model directly.
type ChatRequest struct {
	Prompt      string
	System      string
	ModelID     string
	AllowRemote bool
}

// ChatResult is returned by chat_model.
type ChatResult struct {
	Text      string `json:"text"`
	Model     string `json:"model"`
	Source    string `json:"source"`
	LatencyMS int64  `json:"latencyMs"`
}

// TriggerRequest starts, stops, toggles, or cancels a recording session.
type TriggerRequest struct {
	Feature        string // "transcribe" | "translate" | "rewrite"
	Action         string // "start" | "stop" | "toggle" | "cancel"
	Device         string // optional microphone UID or name
	TargetLanguage string // optional: "en" | "zh" | "ja"
	Deliver        string // optional: "cursor" | "clipboard" | "card"
}

// TriggerResult is returned by trigger_session.
type TriggerResult struct {
	State      string `json:"state"`
	Feature    string `json:"feature,omitempty"`
	Microphone string `json:"microphone,omitempty"`
	Message    string `json:"message,omitempty"`
}

// SessionStatus is returned by session_status and embedded in overview.
type SessionStatus struct {
	State      string `json:"state"` // "idle" | "recording" | "processing" | "done" | "failed"
	Feature    string `json:"feature,omitempty"`
	Microphone string `json:"microphone,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
	LastError  string `json:"lastError,omitempty"`
}
