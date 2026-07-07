package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/model"
)

const apiV3MajorVersion = 3

// searchVisibilityMinorVersion is the 1.x minor that introduced the
// `visibility` filter on search/metadata (the asset-visibility refactor).
const searchVisibilityMinorVersion = 133

// libraryIDNullableMinorVersion is the 1.x minor that made libraryId nullable
// (null = internal upload). Before it, EVERY asset carried the per-user
// upload-library ID, so libraryId cannot distinguish external assets there.
const libraryIDNullableMinorVersion = 106

const maxErrorBodyBytes = 8 * 1024

type ImmichClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	apiV3      bool
	// searchVisibilityUnsupported is set when the server predates the
	// `visibility` search filter; those servers silently strip the field.
	searchVisibilityUnsupported bool
	// libraryIDUnreliable is set when the server predates nullable libraryId;
	// there the field is present on internal uploads too, so it cannot be used
	// to detect external-library assets.
	libraryIDUnreliable bool
}

// CanDetectExternalLibrary reports whether a non-null libraryId reliably
// identifies an external-library asset on this server (Immich 1.106+; assumed
// true when the version is unknown, matching the v3-primary default).
func (c *ImmichClient) CanDetectExternalLibrary() bool {
	return !c.libraryIDUnreliable
}

func NewImmichClient(baseURL, apiKey string) *ImmichClient {
	return &ImmichClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			// No blanket Timeout: it would cover body transfer and make any
			// asset slower than the limit permanently unprocessable. Bound the
			// connection and header phases instead; streams may take as long
			// as they keep moving.
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 10 * time.Second,
				// Also covers the server-side processing gap after a large
				// upload's body is fully sent (checksumming etc.).
				ResponseHeaderTimeout: 5 * time.Minute,
				ExpectContinueTimeout: 1 * time.Second,
			},
			CheckRedirect: newRedirectPolicy(baseURL),
		},
	}
}

// newRedirectPolicy refuses redirects that would leak the x-api-key header:
// Go only strips Authorization/Cookie on cross-origin redirects, so a custom
// header would silently follow to another host or downgrade to plaintext.
func newRedirectPolicy(baseURL string) func(req *http.Request, via []*http.Request) error {
	base, parseErr := url.Parse(baseURL)
	return func(req *http.Request, via []*http.Request) error {
		if parseErr != nil {
			return fmt.Errorf("refusing redirect: cannot parse base URL %q: %w", baseURL, parseErr)
		}
		if !strings.EqualFold(req.URL.Host, base.Host) {
			return fmt.Errorf("refusing redirect to %s: it leaves %s and would forward the API key", req.URL.Host, base.Host)
		}
		if strings.EqualFold(base.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("refusing redirect from https to %s: the API key would be sent in plaintext", req.URL.Scheme)
		}
		return nil
	}
}

func (c *ImmichClient) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	requestURL := c.baseURL + "/api" + path
	req, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	return req, nil
}

// writeMethod is the HTTP verb for the bulk asset update endpoint (/assets).
// Immich v3 deprecated PUT there in favor of PATCH (PUT goes away in v4), but
// PATCH does not exist on legacy servers, so send PATCH only on v3+. This does
// NOT apply to /assets/copy, which has no PATCH alias on v3.0.1.
func (c *ImmichClient) writeMethod() string {
	if c.apiV3 {
		return http.MethodPatch
	}
	return http.MethodPut
}

func (c *ImmichClient) doRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", req.Method, req.URL.Path, err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("%s %s returned %d: %s", req.Method, req.URL.Path, resp.StatusCode, model.SanitizeForTerminal(string(bodyBytes)))
	}
	return resp, nil
}

func (c *ImmichClient) About() (*model.ServerAbout, error) {
	req, err := c.newRequest(http.MethodGet, "/server/about", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var about model.ServerAbout
	if err := c.doJSON(req, &about); err != nil {
		return nil, err
	}
	return &about, nil
}

func (c *ImmichClient) Features() (*model.ServerFeatures, error) {
	req, err := c.newRequest(http.MethodGet, "/server/features", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var features model.ServerFeatures
	if err := c.doJSON(req, &features); err != nil {
		return nil, err
	}
	return &features, nil
}

// ResolveAPIMode selects the API contract to use. mode is "auto" (detect from
// the server version, assuming v3 when the version is unrecognizable),
// "legacy", or "v3". Forced modes must not require the server.about API-key
// permission, so their version probe is best-effort only.
func (c *ImmichClient) ResolveAPIMode(mode string) error {
	switch mode {
	case "legacy", "v3":
		c.apiV3 = mode == "v3"
		if about, err := c.About(); err == nil {
			c.applyServerVersion(about.Version)
		}
		return nil
	default:
		about, err := c.About()
		if err != nil {
			return err
		}
		c.apiV3 = isV3Version(about.Version)
		c.applyServerVersion(about.Version)
		return nil
	}
}

func (c *ImmichClient) applyServerVersion(version string) {
	major, minor, ok := parseVersionMajorMinor(version)
	if !ok {
		return
	}
	c.searchVisibilityUnsupported = major < 1 || (major == 1 && minor < searchVisibilityMinorVersion)
	c.libraryIDUnreliable = major < 1 || (major == 1 && minor < libraryIDNullableMinorVersion)
}

// isV3Version reports whether the server speaks the v3 API contract. v3 is the
// primary supported target, so unrecognizable versions default to v3; point a
// truly old server at the tool with -immich-api legacy.
func isV3Version(version string) bool {
	major, _, ok := parseVersionMajorMinor(version)
	if !ok {
		return true
	}
	return major >= apiV3MajorVersion
}

func parseVersionMajorMinor(version string) (int, int, bool) {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	parts := strings.SplitN(v, ".", 3)
	major, err := strconv.Atoi(stripPreRelease(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	minor := 0
	if len(parts) > 1 {
		if m, err := strconv.Atoi(stripPreRelease(parts[1])); err == nil {
			minor = m
		}
	}
	return major, minor, true
}

func stripPreRelease(part string) string {
	if idx := strings.IndexAny(part, "-+"); idx >= 0 {
		return part[:idx]
	}
	return part
}

func (c *ImmichClient) doJSON(req *http.Request, dest any) error {
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(dest)
}
