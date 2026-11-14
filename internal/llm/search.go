package llm

// How a service searches the web (Preset.Search); "" none.
const (
	// SearchResponses: the Responses API's hosted tool,
	// tools:[{type:"web_search"}] (OpenAI, xAI, Volcengine Ark, DashScope,
	// Codex); only over the Responses API.
	SearchResponses = "responses"
	// SearchAnthropic: Anthropic's server tool web_search_20250305.
	SearchAnthropic = "anthropic"
	// SearchZhipu: Zhipu's chat-completions tool
	// {type:"web_search", web_search:{enable, search_engine}}.
	SearchZhipu = "zhipu"
)

// API choices (ProviderConfig.API).
const (
	APIChat      = "chat"      // the preset's own transport (as "")
	APIResponses = "responses" // the Responses API (Preset.Responses)
)

// maxSearches bounds Anthropic's searches per request (Voxt's 5).
const maxSearches = 5

// SearchSupported reports whether p searches the web over api ("" or
// APIChat: p's transport; APIResponses: the Responses API), for the UI's
// web-search switch. p may be a preset or a resolved one.
func SearchSupported(p Preset, api string) bool {
	responses := p.Transport == TransportResponses || p.Transport == TransportCodex
	if api == APIResponses && !responses {
		if !p.Responses {
			return false // the API is refused (ProviderConfig.Resolve)
		}
		responses = true
	}
	switch p.Search {
	case SearchResponses:
		return responses
	case SearchAnthropic:
		return p.Transport == TransportAnthropic
	case SearchZhipu:
		return !responses && p.Transport == TransportOpenAI
	}
	return false
}

// searches reports whether req searches the web: the backend's search is
// on and req rewrites (dictation and translation never search: latency).
func searches(on bool, req Request) bool { return on && req.Skill == SkillRewrite }

// zhipuSearchTool is Zhipu's web-search tool (Voxt's
// applyOpenAICompatibleSearchConfiguration).
func zhipuSearchTool() []any {
	return []any{map[string]any{
		"type":       "web_search",
		"web_search": map[string]any{"enable": "True", "search_engine": "search_std"},
	}}
}
