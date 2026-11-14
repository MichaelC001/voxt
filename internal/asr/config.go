package asr

// The remote speech services' configuration and HTTP clients (remote
// models, docs/remote-models.md): internal/providers builds a Config from
// the user's setup and its key; the services' packages (openai, doubao,
// aliyun, mimo) take it.

import (
	"net"
	"net/http"
	"net/url"
	"time"
)

// ProxyFunc is an http.Transport's Proxy: the family's network.proxy rule
// (the downloads' rule, internal/models.ProxyFor). nil: the environment's.
type ProxyFunc = func(*http.Request) (*url.URL, error)

// Config is what every remote service's New takes: the user's setup of it
// (settings.Provider) with its secrets from the Keychain.
type Config struct {
	// Endpoint overrides the service's default URL ("": the default).
	Endpoint string
	// Model is the model (Doubao: the resource id); "": the default.
	Model string
	// Key is the API key (Doubao: the Access Token). AppID is Doubao's
	// App ID.
	Key   string
	AppID string
	// Prompt is the user's ASR prompt for this service, sent when a
	// request has none of its own (Caps.Prompt).
	Prompt string
	// Proxy is how requests reach the network (nil: the environment's
	// proxy). HTTPClient, when set, is used instead of one built on it
	// (tests).
	Proxy      ProxyFunc
	HTTPClient *http.Client
}

// proxyOf is p, or the environment's proxy, never for the loopback.
func proxyOf(p ProxyFunc) ProxyFunc {
	if p == nil {
		p = http.ProxyFromEnvironment
	}
	return func(r *http.Request) (*url.URL, error) {
		if loopbackHost(r.URL.Hostname()) {
			return nil, nil
		}
		return p(r)
	}
}

func loopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// NewHTTPClient is an HTTP client through proxy, with dial and TLS
// timeouts. responseHeader bounds the wait for response headers; 0 means
// no bound, for slow streaming answers such as a language model's.
// Requests are otherwise bounded by their context.
func NewHTTPClient(proxy ProxyFunc, responseHeader time.Duration) *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 proxyOf(proxy),
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: responseHeader,
		MaxIdleConnsPerHost:   16,
		ForceAttemptHTTP2:     true,
	}}
}
