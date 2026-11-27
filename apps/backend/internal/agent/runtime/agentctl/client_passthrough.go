package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
)

var ErrPassthroughUnsupported = errors.New("remote helper does not support CLI passthrough. Update Kandev and start a fresh session")
var ErrPassthroughConflict = errors.New("remote CLI ownership changed. Reconnect before retrying")
var ErrPassthroughNotFound = errors.New("remote passthrough process is not available")
var ErrPassthroughCLIUnavailable = errors.New("remote CLI could not start. Check that the selected CLI is installed and executable")

func (c *Client) passthroughJSON(ctx context.Context, method, route string, body, result any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
	}
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/passthrough"+route, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return passthroughResponseError(response, route)
	}
	if result == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1024*1024))
		return err
	}
	return json.NewDecoder(io.LimitReader(response.Body, 16*1024*1024)).Decode(result)
}

func passthroughResponseError(response *http.Response, route string) error {
	switch response.StatusCode {
	case http.StatusNotFound:
		if route == "" {
			return ErrPassthroughUnsupported
		}
		return ErrPassthroughNotFound
	case http.StatusConflict:
		return ErrPassthroughConflict
	case http.StatusBadRequest:
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure)
		if failure.Error == ErrPassthroughCLIUnavailable.Error() {
			return ErrPassthroughCLIUnavailable
		}
	}
	return fmt.Errorf("remote passthrough request failed (HTTP %d)", response.StatusCode)
}

func (c *Client) GetPassthrough(ctx context.Context) (*types.PassthroughSnapshot, error) {
	var snapshot types.PassthroughSnapshot
	if err := c.passthroughJSON(ctx, http.MethodGet, "", nil, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version != types.PassthroughProtocolVersion {
		return nil, ErrPassthroughUnsupported
	}
	return &snapshot, nil
}

func (c *Client) PreparePassthrough(ctx context.Context, request types.PassthroughPrepareRequest) error {
	return c.passthroughJSON(ctx, http.MethodPost, "/prepare", request, nil)
}

func (c *Client) CleanupPassthrough(ctx context.Context, generation uint64) error {
	return c.passthroughJSON(ctx, http.MethodPost, "/cleanup", map[string]uint64{"generation": generation}, nil)
}

func (c *Client) LookupPassthroughStart(ctx context.Context, operationID string) (*types.PassthroughSnapshot, error) {
	var snapshot types.PassthroughSnapshot
	err := c.passthroughJSON(ctx, http.MethodGet, "?operation_id="+url.QueryEscape(operationID), nil, &snapshot)
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (c *Client) StartPassthrough(ctx context.Context, request types.PassthroughStartRequest) (*types.PassthroughSnapshot, error) {
	var snapshot types.PassthroughSnapshot
	if err := c.passthroughJSON(ctx, http.MethodPost, "/start", request, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func passthroughTargetQuery(target types.PassthroughTarget) string {
	return "?process_id=" + url.QueryEscape(target.ProcessID) + "&generation=" + strconv.FormatUint(target.Generation, 10)
}

func (c *Client) WritePassthrough(ctx context.Context, request types.PassthroughInputRequest) (*types.PassthroughInputResult, error) {
	var result types.PassthroughInputResult
	if err := c.passthroughJSON(ctx, http.MethodPost, "/input", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) PassthroughInputReceipt(ctx context.Context, target types.PassthroughTarget, submissionID string) (*types.PassthroughInputResult, error) {
	var result types.PassthroughInputResult
	err := c.passthroughJSON(ctx, http.MethodGet, "/submissions/"+url.PathEscape(submissionID)+passthroughTargetQuery(target), nil, &result)
	return &result, err
}

func (c *Client) ResizePassthrough(ctx context.Context, request types.PassthroughResizeRequest) error {
	return c.passthroughJSON(ctx, http.MethodPost, "/resize", request, nil)
}

func (c *Client) PassthroughNotice(ctx context.Context, target types.PassthroughTarget, data string) error {
	return c.passthroughJSON(ctx, http.MethodPost, "/notice", types.PassthroughInputRequest{PassthroughTarget: target, Data: data}, nil)
}

func (c *Client) StopPassthrough(ctx context.Context, target types.PassthroughTarget) error {
	return c.passthroughJSON(ctx, http.MethodDelete, "/"+url.PathEscape(target.ProcessID)+passthroughTargetQuery(target), nil, nil)
}

func (c *Client) PassthroughBuffer(ctx context.Context, target types.PassthroughTarget) (string, error) {
	var result struct {
		Data string `json:"data"`
	}
	err := c.passthroughJSON(ctx, http.MethodGet, "/buffer"+passthroughTargetQuery(target), nil, &result)
	return result.Data, err
}

func (c *Client) WaitForPassthroughIdle(ctx context.Context, target types.PassthroughTarget) error {
	return c.passthroughJSON(ctx, http.MethodGet, "/idle"+passthroughTargetQuery(target), nil, nil)
}

func (c *Client) StreamPassthrough(ctx context.Context, target types.PassthroughTarget) (*websocket.Conn, error) {
	conn, _, err := c.dialWebSocket(ctx, "/api/v1/passthrough/stream"+passthroughTargetQuery(target), c.wsAuthHeaders())
	return conn, err
}

func (c *Client) StreamPassthroughEvents(ctx context.Context, after uint64) (*websocket.Conn, error) {
	conn, _, err := c.dialWebSocket(ctx, "/api/v1/passthrough/events?after="+strconv.FormatUint(after, 10), c.wsAuthHeaders())
	return conn, err
}
