package llmclient

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type ProviderConfig struct {
	Name string `mapstructure:"-"`

	Model string `mapstructure:"model"`

	APIKey string `mapstructure:"api_key"`

	BaseURL string `mapstructure:"base_url"`

	OrgID string `mapstructure:"org_id"`

	Temperature float64 `mapstructure:"temperature"`
}

type AppConfig struct {
	Common struct {
		ActiveModel string `mapstructure:"active_model"`
	} `mapstructure:"common"`

	Providers map[string]ProviderConfig `mapstructure:",remain"`
}

func LoadConfig(path string) (*AppConfig, error) {

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {

		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {

			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config AppConfig

	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	for name, providerConf := range config.Providers {

		providerConf.Name = name
		config.Providers[name] = providerConf
	}

	return &config, nil
}
