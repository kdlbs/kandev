package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultRegistryURL is the public read-only Registry endpoint.
	DefaultRegistryURL     = "https://registry.modelcontextprotocol.io"
	defaultPageLimit       = 100
	defaultMaxPages        = 1000
	defaultMaxResponseSize = 4 << 20
	defaultMaxTotalSize    = 64 << 20
	defaultMaxEntries      = 10000
	defaultRequestTimeout  = 10 * time.Second
)

// ClientOptions configures the bounded read-only Registry client.
type ClientOptions struct {
	HTTPClient            *http.Client
	MaxResponseBytes      int64
	MaxTotalResponseBytes int64
	MaxEntries            int
	MaxPages              int
	PageLimit             int
	Timeout               time.Duration
}

// ListOptions controls one Registry page request.
type ListOptions struct {
	Cursor         string
	Limit          int
	Search         string
	Version        string
	UpdatedSince   *time.Time
	IncludeDeleted bool
}

// Page is one cursor-paginated Registry response.
type Page struct {
	Entries       []Entry
	NextCursor    string
	Count         int
	responseBytes int64
}

// RegistryHTTPError contains only the upstream status, never its response body.
type RegistryHTTPError struct {
	StatusCode int
}

func (e *RegistryHTTPError) Error() string {
	return fmt.Sprintf("registry request returned HTTP %d", e.StatusCode)
}

var (
	ErrRegistryResponseTooLarge      = errors.New("registry response exceeds configured limit")
	ErrRegistryTotalResponseTooLarge = errors.New("registry responses exceed configured aggregate limit")
	ErrRegistryEntriesTooMany        = errors.New("registry returned too many entries")
	ErrRegistryCursorLoop            = errors.New("registry returned a repeated pagination cursor")
)

// Client reads public Registry metadata without contacting advertised servers.
type Client struct {
	baseURL               *url.URL
	httpClient            *http.Client
	maxResponseBytes      int64
	maxTotalResponseBytes int64
	maxEntries            int
	maxPages              int
	pageLimit             int
	timeout               time.Duration
}

func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	return NewClientWithOptions(baseURL, ClientOptions{HTTPClient: httpClient})
}

func NewClientWithOptions(baseURL string, options ClientOptions) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid registry URL")
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{}
	}
	if options.MaxResponseBytes <= 0 {
		options.MaxResponseBytes = defaultMaxResponseSize
	}
	if options.MaxTotalResponseBytes <= 0 {
		options.MaxTotalResponseBytes = defaultMaxTotalSize
	}
	if options.MaxEntries <= 0 {
		options.MaxEntries = defaultMaxEntries
	}
	if options.MaxPages <= 0 {
		options.MaxPages = defaultMaxPages
	}
	if options.PageLimit <= 0 || options.PageLimit > defaultPageLimit {
		options.PageLimit = defaultPageLimit
	}
	if options.Timeout <= 0 {
		options.Timeout = defaultRequestTimeout
	}
	return &Client{
		baseURL:               parsed,
		httpClient:            options.HTTPClient,
		maxResponseBytes:      options.MaxResponseBytes,
		maxTotalResponseBytes: options.MaxTotalResponseBytes,
		maxEntries:            options.MaxEntries,
		maxPages:              options.MaxPages,
		pageLimit:             options.PageLimit,
		timeout:               options.Timeout,
	}, nil
}

func (c *Client) List(ctx context.Context, options ListOptions) (Page, error) {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/v0.1/servers"
	query := requestURL.Query()
	if options.Cursor != "" {
		query.Set("cursor", options.Cursor)
	}
	limit := options.Limit
	if limit <= 0 || limit > c.pageLimit {
		limit = c.pageLimit
	}
	query.Set("limit", fmt.Sprintf("%d", limit))
	if search := strings.TrimSpace(options.Search); search != "" {
		query.Set("search", search)
	}
	if options.Version != "" {
		query.Set("version", options.Version)
	}
	if options.UpdatedSince != nil {
		query.Set("updated_since", options.UpdatedSince.UTC().Format(time.RFC3339Nano))
		options.IncludeDeleted = true
	}
	if options.IncludeDeleted {
		query.Set("include_deleted", "true")
	}
	requestURL.RawQuery = query.Encode()
	body, err := c.get(ctx, requestURL.String())
	if err != nil {
		return Page{}, err
	}
	page, err := decodePage(body)
	if err != nil {
		return Page{}, err
	}
	page.responseBytes = int64(len(body))
	return page, nil
}

func (c *Client) FetchAll(ctx context.Context, options ListOptions) ([]Entry, error) {
	if options.UpdatedSince != nil {
		options.IncludeDeleted = true
	}
	entries := make([]Entry, 0)
	var totalResponseBytes int64
	seenCursors := make(map[string]struct{})
	for pageNumber := 0; pageNumber < c.maxPages; pageNumber++ {
		page, err := c.List(ctx, options)
		if err != nil {
			return nil, err
		}
		entries = append(entries, page.Entries...)
		totalResponseBytes += page.responseBytes
		if totalResponseBytes > c.maxTotalResponseBytes {
			return nil, ErrRegistryTotalResponseTooLarge
		}
		if len(entries) > c.maxEntries {
			return nil, ErrRegistryEntriesTooMany
		}
		if page.NextCursor == "" {
			return entries, nil
		}
		if _, exists := seenCursors[page.NextCursor]; exists {
			return nil, ErrRegistryCursorLoop
		}
		seenCursors[page.NextCursor] = struct{}{}
		options.Cursor = page.NextCursor
	}
	return nil, fmt.Errorf("registry pagination exceeded %d pages", c.maxPages)
}

func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("create registry request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request registry: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, &RegistryHTTPError{StatusCode: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read registry response: %w", err)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return nil, ErrRegistryResponseTooLarge
	}
	return body, nil
}

type pageEnvelope struct {
	Servers  json.RawMessage `json:"servers"`
	Metadata struct {
		NextCursor      string `json:"nextCursor"`
		NextCursorSnake string `json:"next_cursor"`
		Count           int    `json:"count"`
	} `json:"metadata"`
}

func decodePage(body []byte) (Page, error) {
	var payload pageEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return Page{}, fmt.Errorf("decode registry response: %w", err)
	}
	if len(payload.Servers) == 0 || string(payload.Servers) == "null" {
		return Page{}, errors.New("registry response is missing servers")
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(payload.Servers, &rawEntries); err != nil {
		return Page{}, fmt.Errorf("decode registry servers: %w", err)
	}
	entries := make([]Entry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		entry, err := decodeEntry(raw)
		if err != nil {
			return Page{}, err
		}
		entries = append(entries, entry)
	}
	nextCursor := payload.Metadata.NextCursor
	if nextCursor == "" {
		nextCursor = payload.Metadata.NextCursorSnake
	}
	count := payload.Metadata.Count
	if count == 0 {
		count = len(entries)
	}
	return Page{Entries: entries, NextCursor: nextCursor, Count: count}, nil
}

func decodeEntry(raw json.RawMessage) (Entry, error) {
	var entry Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return Entry{}, fmt.Errorf("decode registry entry: %w", err)
	}
	var wrapper struct {
		Server json.RawMessage `json:"server"`
		Meta   map[string]any  `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return Entry{}, fmt.Errorf("decode registry entry wrapper: %w", err)
	}
	if len(wrapper.Server) > 0 {
		if err := json.Unmarshal(wrapper.Server, &entry); err != nil {
			return Entry{}, fmt.Errorf("decode registry server: %w", err)
		}
	}
	applyEntryMetadata(&entry, wrapper.Meta)
	if entry.Name == "" || entry.Version == "" {
		return Entry{}, errors.New("registry entry is missing name or version")
	}
	return entry, nil
}

func applyEntryMetadata(entry *Entry, metadata map[string]any) {
	if metadata == nil {
		return
	}
	entry.Metadata = metadata
	if status, ok := metadata["status"].(string); ok {
		entry.Status = Status(status)
	}
	if message, ok := metadata["statusMessage"].(string); ok {
		entry.StatusMessage = message
	}
	if publisher, ok := metadata["io.modelcontextprotocol.registry/publisher-provided"].(map[string]any); ok {
		entry.PublisherMetadata = publisher
	}
	if official, ok := metadata["io.modelcontextprotocol.registry/official"].(map[string]any); ok {
		if status, ok := official["status"].(string); ok {
			entry.Status = Status(status)
		}
		if message, ok := official["statusMessage"].(string); ok {
			entry.StatusMessage = message
		}
		if message, ok := official["status_message"].(string); ok {
			entry.StatusMessage = message
		}
	}
}
