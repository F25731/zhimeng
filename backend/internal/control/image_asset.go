package control

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxSiteImageBytes = 1024 * 1024

var imageExtensions = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp",
}

var blockedSVGElements = map[string]bool{
	"script": true, "foreignobject": true, "iframe": true, "object": true,
	"embed": true, "audio": true, "video": true, "image": true, "a": true,
}

var blockedImageAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

func (s *Service) ImportSiteImage(ctx context.Context, source string) (string, error) {
	parsed, err := validateRemoteImageURL(source)
	if err != nil {
		return "", err
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           publicImageDialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		IdleConnTimeout:       10 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many image redirects")
			}
			_, err := validateRemoteImageURL(req.URL.String())
			return err
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", errors.New("invalid image URL")
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp,image/svg+xml")
	req.Header.Set("User-Agent", "Zhimeng-Control/1.0")
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image server returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxSiteImageBytes {
		return "", errors.New("image must be smaller than 1MB")
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxSiteImageBytes+1))
	if err != nil {
		return "", errors.New("failed to read image")
	}
	return s.storeSiteImage(content)
}

func (s *Service) storeSiteImage(content []byte) (string, error) {
	if len(content) == 0 || len(content) > maxSiteImageBytes {
		return "", errors.New("image must be smaller than 1MB")
	}
	extension := ""
	if isSafeSVG(content) {
		extension = ".svg"
	} else {
		extension = imageExtensions[http.DetectContentType(content)]
	}
	if extension == "" {
		return "", errors.New("only PNG, JPEG, WebP and safe SVG images are supported")
	}
	if err := os.MkdirAll("uploads", 0750); err != nil {
		return "", err
	}
	filename := uuid.NewString() + extension
	if err := os.WriteFile(filepath.Join("uploads", filename), content, 0640); err != nil {
		return "", err
	}
	base := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	return base + "/uploads/" + filename, nil
}

func isSafeSVG(content []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	rootSeen := false
	styleDepth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return rootSeen
		}
		if err != nil {
			return false
		}
		switch value := token.(type) {
		case xml.Directive:
			return false
		case xml.ProcInst:
			if !strings.EqualFold(value.Target, "xml") {
				return false
			}
		case xml.StartElement:
			name := strings.ToLower(value.Name.Local)
			if !rootSeen {
				if name != "svg" {
					return false
				}
				rootSeen = true
			}
			if blockedSVGElements[name] {
				return false
			}
			if name == "style" {
				styleDepth++
			}
			for _, attribute := range value.Attr {
				attributeName := strings.ToLower(attribute.Name.Local)
				if strings.HasPrefix(attributeName, "on") {
					return false
				}
				if attributeName == "xmlns" || strings.ToLower(attribute.Name.Space) == "xmlns" {
					continue
				}
				if attributeName == "href" {
					href := strings.TrimSpace(attribute.Value)
					if href != "" && !strings.HasPrefix(href, "#") {
						return false
					}
				}
				if unsafeSVGValue(attribute.Value) {
					return false
				}
			}
		case xml.EndElement:
			if strings.EqualFold(value.Name.Local, "style") && styleDepth > 0 {
				styleDepth--
			}
		case xml.CharData:
			if styleDepth > 0 && unsafeSVGValue(string(value)) {
				return false
			}
		}
	}
}

func unsafeSVGValue(value string) bool {
	value = strings.ToLower(strings.Join(strings.Fields(value), ""))
	for _, blocked := range []string{"javascript:", "vbscript:", "data:", "@import", "http:", "https:"} {
		if strings.Contains(value, blocked) {
			return true
		}
	}
	for index := strings.Index(value, "url("); index >= 0; index = strings.Index(value, "url(") {
		value = value[index+4:]
		if !strings.HasPrefix(strings.TrimLeft(value, "\"'"), "#") {
			return true
		}
		closing := strings.Index(value, ")")
		if closing < 0 {
			return true
		}
		value = value[closing+1:]
	}
	return false
}

func validateRemoteImageURL(source string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, errors.New("请输入有效的 HTTP 或 HTTPS 图片地址")
	}
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		return nil, errors.New("image URL must use port 80 or 443")
	}
	return parsed, nil
}

func publicImageDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, item := range addresses {
		if !isPublicImageAddress(item.IP) {
			continue
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(item.IP.String(), port))
	}
	return nil, errors.New("image URL resolves to a blocked network address")
}

func isPublicImageAddress(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	for _, blocked := range blockedImageAddressRanges {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}
