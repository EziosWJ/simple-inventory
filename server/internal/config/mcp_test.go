package config

import (
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
	if cfg.MCP.Enabled || cfg.MCP.MaxBodyBytes != 65536 || cfg.MCP.MaxOutputBytes != 524288 || cfg.MCP.MaxConcurrent != 4 {
		t.Fatal(cfg.MCP)
	}
	// The higher default stays explicitly bounded.
	cfg.MCP.Enabled = true
	cfg.MCP.AllowedHosts = []string{"127.0.0.1:18300"}
	cfg.MCP.MaxOutputBytes = 1048577
	if e := cfg.Validate(); e == nil {
		t.Fatal("unbounded output limit accepted")
	}
	cfg.MCP.MaxOutputBytes = 524288
	cfg.MCP.Enabled = false
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
	for _, address := range []string{":18300", "0.0.0.0:18300", "192.168.1.10:18300", "localhost:18300"} {
		cfg.HTTP.Address = address
		cfg.MCP.AllowedHosts = []string{"192.168.1.10:18300", "inventory.local:18300", "[fd00::1]:18300"}
		if e := cfg.Validate(); e != nil {
			t.Fatal("LAN listener/host rejected", address, e)
		}
	}
	for _, host := range []string{"*:18300", ":18300", "192.168.1.10:0", "192.168.1.10:65536", "192.168.1.10:http", "bad host:18300"} {
		cfg.MCP.AllowedHosts = []string{host}
		if cfg.Validate() == nil {
			t.Fatal("invalid authority accepted", host)
		}
	}
	cfg.MCP.AllowedHosts = []string{"*"}
	if e := cfg.Validate(); e != nil {
		t.Fatal("development wildcard rejected", e)
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
