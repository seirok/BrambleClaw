package config

import (
	"brambleclaw/internal/logger"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	DefaultConfigFileName = "config"
	DefaultConfigDir      = "config"
	EnvConfigPath         = "BRAMBLECLAW_CONFIG"
	AppName               = "brambleclaw"
	EnvPrefix             = "BRAMBLECLAW"
)

type Loader struct {
	v *viper.Viper
}

func NewLoader() *Loader {
	v := viper.New()
	configureViper(v)
	return &Loader{v: v}
}

func NewLoaderWithPath(path string) *Loader {
	v := viper.New()
	configureViper(v)

	if path != "" {
		v.SetConfigFile(path)
		logger.L().Debug().Str("path", path).Msg("使用显式路径加载配置")
	}

	return &Loader{v: v}
}

func configureViper(v *viper.Viper) {
	v.SetConfigName(DefaultConfigFileName)
	v.SetConfigType("json")
	v.SetEnvPrefix(EnvPrefix)
	v.AutomaticEnv()

	buildViperSearchPaths(v)
}

func buildViperSearchPaths(v *viper.Viper) {
	// 简化搜索路径，保留最常用的几个
	if cwd, err := os.Getwd(); err == nil {
		v.AddConfigPath(filepath.Join(cwd, DefaultConfigDir))
		v.AddConfigPath(cwd)
	}

	if userConfigDir, err := os.UserConfigDir(); err == nil {
		v.AddConfigPath(filepath.Join(userConfigDir, AppName))
	}

	if exePath, err := os.Executable(); err == nil {
		v.AddConfigPath(filepath.Dir(exePath))
	}
}

func (l *Loader) Load() (*Config, string, error) {
	if envPath := os.Getenv(EnvConfigPath); envPath != "" {
		logger.L().Debug().Str("env", EnvConfigPath).Str("path", envPath).Msg("尝试从环境变量加载配置")
		l.v.SetConfigFile(envPath)
		if err := l.v.ReadInConfig(); err == nil {
			logger.L().Debug().Str("path", envPath).Msg("从环境变量加载配置成功")
			return l.unmarshalConfig(), envPath, nil
		}
		logger.L().Warn().Str("env", EnvConfigPath).Str("path", envPath).Msg("环境变量指定的配置文件不存在，尝试其他路径")
	}

	if err := l.v.ReadInConfig(); err == nil {
		logger.L().Debug().Str("path", l.v.ConfigFileUsed()).Msg("从默认搜索路径加载配置成功")
		return l.unmarshalConfig(), l.v.ConfigFileUsed(), nil
	}

	logger.L().Warn().Msg("无法找到配置文件，使用默认配置")
	return getDefaultConfig(), "", nil
}

func (l *Loader) unmarshalConfig() *Config {
	var cfg Config
	if err := l.v.Unmarshal(&cfg); err != nil {
		logger.L().Error().Err(err).Msg("解析配置失败，使用默认配置")
		return getDefaultConfig()
	}

	return ensureAllDefaults(&cfg)
}
