//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"strings"
)

// ThalassaE2EConfig is built from environment variables for live API tests.
type ThalassaE2EConfig struct {
	Enabled bool

	Organisation string
	ThalassaURL  string
	Token        string
	TokenFile    string
	Insecure     bool
	Region       string
}

const (
	EnvThalassaEnabled      = "E2E_THALASSA_ENABLED"
	EnvThalassaOrganisation = "E2E_THALASSA_ORGANISATION"
	EnvThalassaURL          = "E2E_THALASSA_URL"
	EnvThalassaToken        = "E2E_THALASSA_TOKEN"
	EnvThalassaTokenFile    = "E2E_THALASSA_TOKEN_FILE"
	EnvThalassaInsecure     = "E2E_THALASSA_INSECURE"
	EnvThalassaRegion       = "E2E_THALASSA_REGION"
)

// LoadThalassaE2EConfig reads configuration from the environment.
func LoadThalassaE2EConfig() (ThalassaE2EConfig, error) {
	cfg := ThalassaE2EConfig{
		Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv(EnvThalassaEnabled)), "true"),
	}
	if !cfg.Enabled {
		return cfg, nil
	}
	cfg.Organisation = strings.TrimSpace(os.Getenv(EnvThalassaOrganisation))
	cfg.ThalassaURL = strings.TrimSpace(os.Getenv(EnvThalassaURL))
	cfg.Token = strings.TrimSpace(os.Getenv(EnvThalassaToken))
	cfg.TokenFile = strings.TrimSpace(os.Getenv(EnvThalassaTokenFile))
	cfg.Insecure = parseBoolEnv(EnvThalassaInsecure)
	cfg.Region = strings.TrimSpace(os.Getenv(EnvThalassaRegion))
	if cfg.ThalassaURL == "" {
		cfg.ThalassaURL = "https://api.thalassa.cloud/"
	}
	if cfg.Region == "" {
		cfg.Region = "nl-01"
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c *ThalassaE2EConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	var missing []string
	if c.Organisation == "" {
		missing = append(missing, EnvThalassaOrganisation)
	}
	if c.Token == "" && c.TokenFile == "" {
		missing = append(missing, EnvThalassaToken+" or "+EnvThalassaTokenFile)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env when %s=true: %s", EnvThalassaEnabled, strings.Join(missing, ", "))
	}
	return nil
}

func parseBoolEnv(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "true" || v == "1" || v == "yes"
}
