package config

import (
	"strings"
	"testing"
)

func TestMCPConfigDefaultsAndBounds(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `database:
  driver: sqlite
  url: /tmp/config-only-mcp-test.db
jwt:
  secret: config-only-test
`)
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_CONFIG_PROFILE", "")
	t.Setenv("APP_DATABASE__USERNAME", "")
	t.Setenv("APP_DATABASE__PASSWORD", "")
	cfg, e := LoadFromDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.MCP.Enabled || cfg.MCP.MaxBodyBytes != 65536 || cfg.MCP.MaxConcurrent != 4 {
		t.Fatal(cfg.MCP)
	}
	t.Setenv("APP_MCP__ENABLED", "true")
	t.Setenv("APP_HTTP__ADDRESS", "127.0.0.1:18300")
	t.Setenv("APP_MCP__ALLOWED_HOSTS", "127.0.0.1:18300,localhost:18300")
	cfg, e = LoadFromDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	if len(cfg.MCP.AllowedHosts) != 2 {
		t.Fatal(cfg.MCP)
	}
	for _, address := range []string{":18300", "0.0.0.0:18300", "localhost:18300"} {
		cfg.HTTP.Address = address
		if e := cfg.Validate(); e == nil || !strings.Contains(e.Error(), "loopback") {
			t.Fatal("accepted nonliteral listener", address, e)
		}
	}
	cfg.HTTP.Address = "127.0.0.1:18300"
	cfg.MCP.MaxConcurrent = 33
	if cfg.Validate() == nil {
		t.Fatal("unbounded concurrency")
	}
	cfg.MCP.MaxConcurrent = 4
	cfg.MCP.AllowedOrigins = []string{"*"}
	if cfg.Validate() == nil {
		t.Fatal("wildcard origin")
	}
}
