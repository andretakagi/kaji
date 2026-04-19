package caddy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
)

type DomainToggles struct {
	ForceHTTPS  bool            `json:"force_https"`
	Compression bool            `json:"compression"`
	Headers     HeadersConfig   `json:"headers"`
	BasicAuth   BasicAuth       `json:"basic_auth"`
	AccessLog   string          `json:"access_log"`
	IPFiltering IPFilteringOpts `json:"ip_filtering"`
}

type ReverseProxyConfig struct {
	Upstream          string         `json:"upstream"`
	TLSSkipVerify     bool           `json:"tls_skip_verify"`
	WebSocketPassthru bool           `json:"websocket_passthrough"`
	LoadBalancing     LoadBalancing  `json:"load_balancing"`
	RequestHeaders    RequestHeaders `json:"request_headers"`
}

type StaticResponseConfig struct {
	StatusCode string              `json:"status_code"`
	Body       string              `json:"body"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Close      bool                `json:"close"`
}

func generateOpaqueID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return prefix + hex.EncodeToString(b)
}

func GenerateDomainID() string {
	return generateOpaqueID("dom_")
}

func GenerateRuleID() string {
	return generateOpaqueID("rule_")
}

func CaddyRouteID(ruleID string) string {
	return "kaji_" + ruleID
}

// MergeToggles returns the override toggles if non-nil, otherwise the domain defaults.
// Full-replacement semantics: no field-level merge.
func MergeToggles(defaults DomainToggles, overrides *DomainToggles) DomainToggles {
	if overrides != nil {
		return *overrides
	}
	return defaults
}

func ParseReverseProxyConfig(raw json.RawMessage) (ReverseProxyConfig, error) {
	var cfg ReverseProxyConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ReverseProxyConfig{}, err
	}
	return cfg, nil
}

func MarshalReverseProxyConfig(cfg ReverseProxyConfig) (json.RawMessage, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func ParseStaticResponseConfig(raw json.RawMessage) (StaticResponseConfig, error) {
	var cfg StaticResponseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return StaticResponseConfig{}, err
	}
	return cfg, nil
}

func MarshalStaticResponseConfig(cfg StaticResponseConfig) (json.RawMessage, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
