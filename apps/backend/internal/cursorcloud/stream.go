package cursorcloud

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxSSEFrameBytes = 1 << 20

type StreamEvent struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type StreamResult struct {
	Retention time.Duration
}

func (c *Client) StreamRun(ctx context.Context, agentID, runID, lastEventID string, handle func(StreamEvent) error) (StreamResult, error) {
	if c.configError != nil {
		return StreamResult{}, c.configError
	}
	if c.apiKey == "" {
		return StreamResult{}, errors.New("cursor cloud API key is required")
	}
	if !agentIDPattern.MatchString(agentID) || runID == "" {
		return StreamResult{}, errors.New("cursor cloud agent and run IDs are required")
	}
	if handle == nil {
		return StreamResult{}, errors.New("cursor cloud stream handler is required")
	}
	response, err := c.openRunStream(ctx, agentID, runID, lastEventID)
	if err != nil {
		return StreamResult{}, err
	}
	defer func() { _ = response.Body.Close() }()
	return consumeRunStream(response, handle, c.now())
}

func (c *Client) openRunStream(ctx context.Context, agentID, runID, lastEventID string) (*http.Response, error) {
	path := "/v1/agents/" + url.PathEscape(agentID) + "/runs/" + url.PathEscape(runID) + "/stream"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create Cursor Cloud stream request: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := c.streamClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("cursor cloud stream disconnected: %w", err)
	}
	return response, nil
}

func consumeRunStream(response *http.Response, handle func(StreamEvent) error, now time.Time) (StreamResult, error) {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, err := readBounded(response.Body, maxResponseBytes)
		if err != nil {
			return StreamResult{}, fmt.Errorf("read Cursor Cloud stream error: %w", err)
		}
		return StreamResult{}, decodeAPIError(response.StatusCode, response.Header, body, now)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "text/event-stream" {
		return StreamResult{}, &ContractError{Detail: "stream response did not use text/event-stream content type"}
	}
	retention, err := streamRetention(response.Header.Get("X-Cursor-Stream-Retention-Seconds"))
	if err != nil {
		return StreamResult{}, &ContractError{Detail: "stream response contained an invalid retention header"}
	}
	if err := parseSSE(response.Body, handle); err != nil {
		return StreamResult{}, err
	}
	return StreamResult{Retention: retention}, nil
}

type sseFrameParser struct {
	eventType  string
	eventID    string
	data       []string
	frameBytes int
	handle     func(StreamEvent) error
}

func parseSSE(reader io.Reader, handle func(StreamEvent) error) error {
	buffered := bufio.NewReaderSize(reader, 4096)
	parser := sseFrameParser{handle: handle}
	for {
		line, err := readSSELine(buffered, maxSSEFrameBytes)
		if len(line) > 0 {
			if lineErr := parser.consumeLine(line); lineErr != nil {
				return lineErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return parser.dispatch()
			}
			return &ContractError{Detail: "stream event framing exceeded the configured limit"}
		}
	}
}

func (p *sseFrameParser) consumeLine(line []byte) error {
	p.frameBytes += len(line)
	if p.frameBytes > maxSSEFrameBytes {
		return &ContractError{Detail: "stream event exceeded the configured limit"}
	}
	lineText := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
	if lineText == "" {
		return p.dispatch()
	}
	if strings.HasPrefix(lineText, ":") {
		return nil
	}
	field, value, found := strings.Cut(lineText, ":")
	if !found {
		value = ""
	}
	value = strings.TrimPrefix(value, " ")
	switch field {
	case "event":
		p.eventType = value
	case "data":
		p.data = append(p.data, value)
	case "id":
		if !strings.ContainsRune(value, '\x00') {
			p.eventID = value
		}
	}
	return nil
}

func (p *sseFrameParser) dispatch() error {
	defer p.reset()
	if len(p.data) == 0 {
		return nil
	}
	payload := []byte(strings.Join(p.data, "\n"))
	if !json.Valid(payload) {
		return &ContractError{Detail: "stream event contained malformed JSON"}
	}
	if p.eventType == "" {
		p.eventType = "message"
	}
	event := StreamEvent{ID: p.eventID, Type: p.eventType, Data: append(json.RawMessage(nil), payload...)}
	return p.handle(event)
}

func (p *sseFrameParser) reset() {
	p.eventType = ""
	p.eventID = ""
	p.data = nil
	p.frameBytes = 0
}

func readSSELine(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > maxBytes {
			return nil, fmt.Errorf("SSE line too large")
		}
		line = append(line, part...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}

func streamRetention(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 || seconds > int64((time.Duration(1<<63-1))/time.Second) {
		return 0, errors.New("invalid retention duration")
	}
	return time.Duration(seconds) * time.Second, nil
}
