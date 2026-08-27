package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/majorfi/immich-exif/model"
)

// ListStacks returns every stack of the key's owner. The endpoint is
// unpaginated and inlines a full asset DTO per member, so the response grows
// with the library; it is only used by the one-shot -repair-stacks mode.
// Each stack's assets list is already filtered server-side to live members
// with timeline or archive visibility.
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

// UpdateStackPrimary re-points a stack at another of its members. Immich only
// shows a stack's primary in the timeline, so a stack whose primary is trashed
// disappears entirely until this runs.
//
// Unlike /assets/copy, this route has both verbs registered (@Put deprecated in
// v3, @Patch since v3.0.0), so writeMethod() applies.
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
