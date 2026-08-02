package main

import "testing"

func TestLoadConfigDefaultsToSiteSchemaPrefix(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/site")
	t.Setenv("CONTROL_CENTER_URL", "https://open.juheai.club")
	t.Setenv("CONTROL_SITE_ID", "site-id")
	t.Setenv("CONTROL_SITE_TOKEN", "12345678901234567890123456789012")
	t.Setenv("SITE_DATABASE_TABLE_PREFIX", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.tablePrefix != "vozeb_pro_" {
		t.Fatalf("unexpected table prefix: %q", cfg.tablePrefix)
	}
	r := reporter{cfg: cfg}
	if got := r.table("users"); got != `"vozeb_pro_users"` {
		t.Fatalf("unexpected table identifier: %s", got)
	}
}

func TestLoadConfigRejectsUnsafeTablePrefix(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/site")
	t.Setenv("CONTROL_CENTER_URL", "https://open.juheai.club")
	t.Setenv("CONTROL_SITE_ID", "site-id")
	t.Setenv("CONTROL_SITE_TOKEN", "12345678901234567890123456789012")
	t.Setenv("SITE_DATABASE_TABLE_PREFIX", `vozeb_pro_";DROP TABLE users;--`)

	if _, err := loadConfig(); err == nil {
		t.Fatal("unsafe table prefix was accepted")
	}
}
