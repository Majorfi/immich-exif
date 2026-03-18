package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ImmichClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
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

func (c *ImmichClient) Ping() error {
	req, err := c.newRequest(http.MethodGet, "/server/about", nil)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (c *ImmichClient) doJSON(req *http.Request, dest any) error {
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(dest)
}
