package control

import (
	"net"
	"strings"
	"testing"
)

func TestSafeSVG(t *testing.T) {
	safe := `<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="a"/></defs><path fill="url(#a)" d="M0 0h10v10z"/></svg>`
	if !isSafeSVG([]byte(safe)) {
		t.Fatal("expected ordinary SVG to be accepted")
	}
	for _, source := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><path onclick="alert(1)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.com/a.png"/></svg>`,
		`<!DOCTYPE svg><svg xmlns="http://www.w3.org/2000/svg"/>`,
	} {
		if isSafeSVG([]byte(source)) {
			t.Fatalf("expected unsafe SVG to be rejected: %s", strings.TrimSpace(source))
		}
	}
}

func TestRemoteImageURLValidation(t *testing.T) {
	for _, value := range []string{"https://example.com/logo.png", "http://images.example.com/path?id=1"} {
		if _, err := validateRemoteImageURL(value); err != nil {
			t.Fatalf("valid image URL %q was rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "data:image/png;base64,abc", "ftp://example.com/logo.png", "https://user:pass@example.com/logo.png", "https://example.com:8080/logo.png"} {
		if _, err := validateRemoteImageURL(value); err == nil {
			t.Fatalf("unsafe image URL %q was accepted", value)
		}
	}
}

func TestPublicImageAddressValidation(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1"} {
		if isPublicImageAddress(net.ParseIP(value)) {
			t.Fatalf("private image address %s was accepted", value)
		}
	}
	if !isPublicImageAddress(net.ParseIP("8.8.8.8")) {
		t.Fatal("public image address was rejected")
	}
}
