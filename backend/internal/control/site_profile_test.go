package control

import (
	"strings"
	"testing"
)

func TestNormalizeSiteBootstrapProfileKeepsBlankValuesBlank(t *testing.T) {
	profile, err := normalizeSiteBootstrapProfile(siteBootstrapProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Title != "" || profile.LogoURL != "" || profile.SEOTitle != "" || profile.FooterCopyright != "" {
		t.Fatalf("blank profile was unexpectedly populated: %#v", profile)
	}
	if profile.HomeShowcaseMode != "custom" || len(profile.HomeShowcaseItems) != 0 || len(profile.FriendLinks) != 0 {
		t.Fatalf("blank collections were unexpectedly populated: %#v", profile)
	}
	for key, setting := range profile.Socials {
		if setting.Enabled || setting.URL != "" {
			t.Fatalf("social %s was unexpectedly enabled: %#v", key, setting)
		}
	}
}

func TestNormalizeSiteBootstrapProfilePreservesConfiguredValues(t *testing.T) {
	profile, err := normalizeSiteBootstrapProfile(siteBootstrapProfile{
		Title:       "  测试站  ",
		TermsURL:    "/terms",
		FriendLinks: []siteFriendLink{{Label: "文档", URL: "https://docs.example.com"}},
		Socials:     map[string]siteSocialSetting{"email": {URL: "mailto:admin@example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Title != "测试站" || profile.TermsURL != "/terms" || len(profile.FriendLinks) != 1 {
		t.Fatalf("profile values were not preserved: %#v", profile)
	}
	if !profile.Socials["email"].Enabled || profile.Socials["email"].URL != "mailto:admin@example.com" {
		t.Fatalf("email setting was not normalized: %#v", profile.Socials["email"])
	}
}

func TestVerifyAppliedSiteProfileRejectsUpstreamBranding(t *testing.T) {
	expected, err := normalizeSiteBootstrapProfile(siteBootstrapProfile{})
	if err != nil {
		t.Fatal(err)
	}
	actual := expected
	actual.FriendLinks = []siteFriendLink{{ID: "legacy", Label: "legacy", URL: "https://www.vozeb.com", Enabled: true}}
	err = verifyAppliedSiteProfile(expected, actual)
	if err == nil || !strings.Contains(err.Error(), "forbidden upstream branding") {
		t.Fatalf("expected upstream branding rejection, got %v", err)
	}
}
