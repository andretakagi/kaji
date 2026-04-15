package snapshot

import (
	"encoding/json"
)

// Data is the envelope stored in each snapshot file. Legacy snapshots
// (pre-upgrade) contain only raw Caddy JSON with no envelope.
type Data struct {
	KajiVersion string          `json:"kaji_version,omitempty"`
	CaddyConfig json.RawMessage `json:"caddy_config"`
	AppConfig   json.RawMessage `json:"app_config,omitempty"`
}

// ParseData reads snapshot file bytes and returns structured Data.
// It detects legacy format (raw Caddy JSON without envelope) by
// checking for the caddy_config key.
func ParseData(raw []byte) (*Data, error) {
	var probe struct {
		CaddyConfig json.RawMessage `json:"caddy_config"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}

	if probe.CaddyConfig != nil {
		var d Data
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		return &d, nil
	}

	return &Data{
		CaddyConfig: raw,
	}, nil
}
