package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/majorfi/immich-exif/model"
)

func (c *ImmichClient) ListStacks() ([]model.StackResponse, error) {
	req, err := c.newRequest(http.MethodGet, "/stacks", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	var stacks []model.StackResponse
	if err := c.doJSON(req, &stacks); err != nil {
		return nil, err
	}
	return stacks, nil
}

func (c *ImmichClient) UpdateStackPrimary(stackID, primaryAssetID string) error {
	body := model.UpdateStackRequest{PrimaryAssetID: primaryAssetID}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := c.newRequest(c.writeMethod(), "/stacks/"+url.PathEscape(stackID), bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}
