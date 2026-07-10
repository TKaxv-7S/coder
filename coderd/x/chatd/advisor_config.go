package chatd

import (
	"encoding/json"

	"github.com/coder/coder/v2/codersdk"
)

const advisorConfigStorageVersion = 1

type storedAdvisorConfig struct {
	codersdk.AdvisorConfig
	Version int `json:"_version,omitempty"`
}

// EncodeAdvisorConfig encodes advisor configuration for site-config storage.
func EncodeAdvisorConfig(config codersdk.AdvisorConfig) ([]byte, error) {
	return json.Marshal(storedAdvisorConfig{
		AdvisorConfig: config,
		Version:       advisorConfigStorageVersion,
	})
}

// DecodeAdvisorConfig decodes advisor configuration from site-config storage.
func DecodeAdvisorConfig(data []byte) (codersdk.AdvisorConfig, error) {
	var stored storedAdvisorConfig
	if err := json.Unmarshal(data, &stored); err != nil {
		return codersdk.AdvisorConfig{}, err
	}
	if stored.Version != advisorConfigStorageVersion {
		stored.ReasoningEffort = nil
	}
	return stored.AdvisorConfig, nil
}
