package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/model"
)

const apiV3MajorVersion = 3

type ImmichClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	apiV3      bool
}

func NewImmichClient(baseURL, apiKey string) *ImmichClient {
	return &ImmichClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (c *ImmichClient) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := c.baseURL + "/api" + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	return req, nil
}

func (c *ImmichClient) doRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", req.Method, req.URL.Path, err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s %s returned %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(bodyBytes))
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

// ResolveAPIMode verifies connectivity and selects the API contract to use.
// mode is "auto" (detect from server version), "legacy", or "v3".
func (c *ImmichClient) ResolveAPIMode(mode string) error {
	about, err := c.About()
	if err != nil {
		return err
	}
	switch mode {
	case "legacy":
		c.apiV3 = false
	case "v3":
		c.apiV3 = true
	default:
		c.apiV3 = isV3Version(about.Version)
	}
	return nil
}

func isV3Version(version string) bool {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if dot := strings.IndexByte(v, '.'); dot >= 0 {
		v = v[:dot]
	}
	major, err := strconv.Atoi(v)
	if err != nil {
		return false
	}
	return major >= apiV3MajorVersion
}

func (c *ImmichClient) doJSON(req *http.Request, dest any) error {
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(dest)
}
