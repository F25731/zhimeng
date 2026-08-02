package control

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/F25731/zhimeng/backend/internal/config"
)

func TestProvisionCodeFormat(t *testing.T) {
	code, err := newCode()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(code, "-")
	if len(parts) != 5 || parts[0] != "SITE" {
		t.Fatalf("unexpected code format: %q", code)
	}
	for _, part := range parts[1:] {
		if len(part) != 4 {
			t.Fatalf("unexpected code group %q", part)
		}
	}
}

func TestDomainPrefixPolicy(t *testing.T) {
	valid := []string{"abc", "brand-01", "a1b"}
	invalid := []string{"ab", "-abc", "abc-", "ABC", "a_b", "contains.dot"}
	for _, value := range valid {
		if !slugPattern.MatchString(value) || reservedPrefixes[value] {
			t.Errorf("expected valid prefix %q", value)
		}
	}
	for _, value := range invalid {
		if slugPattern.MatchString(value) {
			t.Errorf("expected invalid prefix %q", value)
		}
	}
	if !reservedPrefixes["open"] || !reservedPrefixes["admin"] {
		t.Fatal("required reserved prefixes are missing")
	}
}

func TestLogoURLMustBelongToControlCenter(t *testing.T) {
	service := &Service{cfg: config.Config{PublicBaseURL: "https://open.juheai.club"}}
	for _, value := range []string{"", "/uploads/logo.png", "https://open.juheai.club/uploads/logo.webp"} {
		if !service.validLogoURL(value) {
			t.Errorf("expected allowed logo URL %q", value)
		}
	}
	for _, value := range []string{"https://example.com/logo.png", "file:///etc/passwd", "https://open.juheai.club/other/logo.png"} {
		if service.validLogoURL(value) {
			t.Errorf("expected rejected logo URL %q", value)
		}
	}
}

func TestSiteSigningString(t *testing.T) {
	hash := sha256.Sum256([]byte(`{"ok":true}`))
	got := siteSigningString("site-id", "1722500000", "nonce", "POST", "/api/internal/sites/heartbeat", hash[:])
	want := "site-id\n1722500000\nnonce\nPOST\n/api/internal/sites/heartbeat\n4062edaf750fb8074e7e83e0c9028c94e32468a8b6f1614774328ef045150f93"
	if got != want {
		t.Fatalf("unexpected signing string: %q", got)
	}
}
