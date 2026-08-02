package control

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type siteBootstrapProfile struct {
	Title             string                       `json:"title"`
	LogoURL           string                       `json:"logoUrl"`
	IconURL           string                       `json:"iconUrl"`
	SEOTitle          string                       `json:"seoTitle"`
	SEODescription    string                       `json:"seoDescription"`
	SEOKeywords       string                       `json:"seoKeywords"`
	FooterCopyright   string                       `json:"footerCopyright"`
	TermsURL          string                       `json:"termsUrl"`
	PrivacyURL        string                       `json:"privacyUrl"`
	HomeShowcaseMode  string                       `json:"homeShowcaseMode"`
	HomeShowcaseItems []map[string]any             `json:"homeShowcaseItems"`
	FriendLinks       []siteFriendLink             `json:"friendLinks"`
	Socials           map[string]siteSocialSetting `json:"socials"`
}

type siteFriendLink struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

type siteSocialSetting struct {
	Enabled bool   `json:"enabled"`
	Label   string `json:"label"`
	URL     string `json:"url"`
}

func normalizeSiteBootstrapProfile(input siteBootstrapProfile) (siteBootstrapProfile, error) {
	profile := siteBootstrapProfile{
		Title:             strings.TrimSpace(input.Title),
		LogoURL:           strings.TrimSpace(input.LogoURL),
		IconURL:           strings.TrimSpace(input.IconURL),
		SEOTitle:          strings.TrimSpace(input.SEOTitle),
		SEODescription:    strings.TrimSpace(input.SEODescription),
		SEOKeywords:       strings.TrimSpace(input.SEOKeywords),
		FooterCopyright:   strings.TrimSpace(input.FooterCopyright),
		TermsURL:          strings.TrimSpace(input.TermsURL),
		PrivacyURL:        strings.TrimSpace(input.PrivacyURL),
		HomeShowcaseMode:  "custom",
		HomeShowcaseItems: []map[string]any{},
		FriendLinks:       []siteFriendLink{},
		Socials:           map[string]siteSocialSetting{},
	}
	limits := map[string]struct {
		value string
		max   int
	}{
		"site title": {profile.Title, 40}, "SEO title": {profile.SEOTitle, 72},
		"SEO description": {profile.SEODescription, 180}, "SEO keywords": {profile.SEOKeywords, 240},
		"footer copyright": {profile.FooterCopyright, 120},
	}
	for name, item := range limits {
		if utf8.RuneCountInString(item.value) > item.max {
			return siteBootstrapProfile{}, errors.New(name + " is too long")
		}
	}
	for name, value := range map[string]string{"logo URL": profile.LogoURL, "icon URL": profile.IconURL} {
		if !validProfileURL(value, false) {
			return siteBootstrapProfile{}, errors.New("invalid " + name)
		}
	}
	for name, value := range map[string]string{"terms URL": profile.TermsURL, "privacy URL": profile.PrivacyURL} {
		if !validProfileURL(value, false) {
			return siteBootstrapProfile{}, errors.New("invalid " + name)
		}
	}
	for _, link := range input.FriendLinks {
		label, value := strings.TrimSpace(link.Label), strings.TrimSpace(link.URL)
		if label == "" && value == "" {
			continue
		}
		if label == "" || utf8.RuneCountInString(label) > 32 || !validProfileURL(value, false) {
			return siteBootstrapProfile{}, errors.New("invalid friend link")
		}
		profile.FriendLinks = append(profile.FriendLinks, siteFriendLink{ID: uuid.NewString(), Label: label, URL: value, Enabled: true})
		if len(profile.FriendLinks) == 12 {
			break
		}
	}
	labels := map[string]string{"email": "邮箱联系", "telegram": "Telegram", "x": "X", "instagram": "Instagram"}
	for key, label := range labels {
		setting := input.Socials[key]
		value := strings.TrimSpace(setting.URL)
		if value != "" && !validProfileURL(value, key == "email") {
			return siteBootstrapProfile{}, errors.New("invalid " + key + " URL")
		}
		profile.Socials[key] = siteSocialSetting{Enabled: value != "", Label: label, URL: value}
	}
	return profile, nil
}

func validProfileURL(value string, allowMailto bool) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	if allowMailto && parsed.Scheme == "mailto" {
		return parsed.Opaque != ""
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func (profile siteBootstrapProfile) settingsPayload() map[string]any {
	return map[string]any{
		"title": profile.Title, "logoUrl": profile.LogoURL, "iconUrl": profile.IconURL,
		"seoTitle": profile.SEOTitle, "seoDescription": profile.SEODescription, "seoKeywords": profile.SEOKeywords,
		"footerCopyright": profile.FooterCopyright, "termsUrl": profile.TermsURL, "privacyUrl": profile.PrivacyURL,
		"homeShowcaseMode": profile.HomeShowcaseMode, "homeShowcaseItems": profile.HomeShowcaseItems,
		"friendLinks": profile.FriendLinks, "socials": profile.Socials,
	}
}
