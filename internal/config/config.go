package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port          int
	BindHost      string
	APIKey        string
	UpstreamBase  string
	BootstrapPath string
	ChatPath      string
	Fingerprint   string
	Debug         bool
}

func Load() *Config {
	base := strings.TrimRight(getEnv("MIMO_FREE_BASE_URL", "https://api.xiaomimimo.com"), "/")
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = generateAPIKey()
	}

	return &Config{
		Port:          getEnvInt("MIMO2API_PORT", 10000),
		BindHost:      getEnv("BIND_HOST", "0.0.0.0"),
		APIKey:        apiKey,
		UpstreamBase:  base,
		BootstrapPath: base + "/api/free-ai/bootstrap",
		ChatPath:      base + "/api/free-ai/openai/chat",
		Fingerprint:   os.Getenv("MIMO_FINGERPRINT"),
		Debug:         getEnvBool("MIMO2API_DEBUG", false),
	}
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "sk-" + hex.EncodeToString(b)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "1" || v == "true" || v == "yes" {
		return true
	}
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return fallback
}