package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"sync"
	"unicode/utf16"
	"unicode/utf8"
)

type DocumentSnapshot struct {
	URI        string
	LanguageID string
	Text       string
	Version    uint64
	References int
}

type openDocument struct {
	uri        string
	languageID string
	text       string
	version    uint64
	references map[uint64]struct{}
}

type documentBroker struct {
	mu       sync.Mutex
	upstream featureUpstream
	docs     map[string]*openDocument
	save     documentSaveCapability
}

type documentSaveCapability struct {
	enabled     bool
	includeText bool
}

func newDocumentBroker(upstream featureUpstream) *documentBroker {
	return &documentBroker{upstream: upstream, docs: make(map[string]*openDocument)}
}

func (b *documentBroker) SetCapabilities(raw json.RawMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.save = parseDocumentSaveCapability(raw)
}

func (b *documentBroker) Handle(attachmentID uint64, method string, params json.RawMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch method {
	case methodDidOpen:
		return b.open(attachmentID, params)
	case methodDidChange:
		return b.change(attachmentID, params)
	case methodDidSave:
		return b.saveDocument(attachmentID, params)
	case methodDidClose:
		return b.close(attachmentID, params)
	default:
		return fmt.Errorf("unsupported document notification: %s", method)
	}
}

func (b *documentBroker) open(attachmentID uint64, raw json.RawMessage) error {
	var params struct {
		TextDocument struct {
			URI        string `json:"uri"`
			LanguageID string `json:"languageId"`
			Text       string `json:"text"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode didOpen: %w", err)
	}
	uri, err := canonicalDocumentURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	if document := b.docs[uri]; document != nil {
		document.references[attachmentID] = struct{}{}
		return nil
	}
	params.TextDocument.URI = uri
	payload, _ := json.Marshal(map[string]any{
		fieldTextDocument: map[string]any{
			fieldURI:        uri,
			fieldLanguageID: params.TextDocument.LanguageID,
			fieldVersion:    1,
			fieldText:       params.TextDocument.Text,
		},
	})
	if err := b.upstream.Notify(methodDidOpen, payload); err != nil {
		return err
	}
	b.docs[uri] = &openDocument{
		uri:        uri,
		languageID: params.TextDocument.LanguageID,
		text:       params.TextDocument.Text,
		version:    1,
		references: map[uint64]struct{}{attachmentID: {}},
	}
	return nil
}

func (b *documentBroker) change(attachmentID uint64, raw json.RawMessage) error {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		ContentChanges []json.RawMessage `json:"contentChanges"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode didChange: %w", err)
	}
	uri, err := canonicalDocumentURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	document := b.docs[uri]
	if document == nil {
		return errors.New("didChange received for unopened document")
	}
	if _, attached := document.references[attachmentID]; !attached {
		return errors.New("didChange attachment does not own document reference")
	}
	text := document.text
	for _, change := range params.ContentChanges {
		text, err = applyContentChange(text, change)
		if err != nil {
			return err
		}
	}
	if len(params.ContentChanges) == 0 {
		return nil
	}
	nextVersion := document.version + 1
	payload, _ := json.Marshal(map[string]any{
		fieldTextDocument:   map[string]any{fieldURI: uri, fieldVersion: nextVersion},
		fieldContentChanges: params.ContentChanges,
	})
	if err := b.upstream.Notify(methodDidChange, payload); err != nil {
		return err
	}
	document.text = text
	document.version = nextVersion
	return nil
}

func (b *documentBroker) saveDocument(attachmentID uint64, raw json.RawMessage) error {
	if !b.save.enabled {
		return nil
	}
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode didSave: %w", err)
	}
	uri, err := canonicalDocumentURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	document := b.docs[uri]
	if document == nil {
		return errors.New("didSave received for unopened document")
	}
	if _, attached := document.references[attachmentID]; !attached {
		return errors.New("didSave attachment does not own document reference")
	}
	payload := map[string]any{fieldTextDocument: map[string]any{fieldURI: uri}}
	if b.save.includeText && params.Text != nil && *params.Text == document.text {
		payload[fieldText] = *params.Text
	}
	encoded, _ := json.Marshal(payload)
	return b.upstream.Notify(methodDidSave, encoded)
}

func (b *documentBroker) close(attachmentID uint64, raw json.RawMessage) error {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("decode didClose: %w", err)
	}
	uri, err := canonicalDocumentURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	return b.releaseReference(attachmentID, uri)
}

func (b *documentBroker) ReleaseAttachment(attachmentID uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	uris := make([]string, 0)
	for uri, document := range b.docs {
		if _, attached := document.references[attachmentID]; attached {
			uris = append(uris, uri)
		}
	}
	sort.Strings(uris)
	for _, uri := range uris {
		_ = b.releaseReference(attachmentID, uri)
	}
}

func (b *documentBroker) releaseReference(attachmentID uint64, uri string) error {
	document := b.docs[uri]
	if document == nil {
		return nil
	}
	if _, attached := document.references[attachmentID]; !attached {
		return nil
	}
	delete(document.references, attachmentID)
	if len(document.references) != 0 {
		return nil
	}
	delete(b.docs, uri)
	payload, _ := json.Marshal(map[string]any{
		fieldTextDocument: map[string]any{fieldURI: uri},
	})
	return b.upstream.Notify(methodDidClose, payload)
}

func (b *documentBroker) Snapshot(uri string) (DocumentSnapshot, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	canonical, err := canonicalDocumentURI(uri)
	if err != nil {
		return DocumentSnapshot{}, false
	}
	document := b.docs[canonical]
	if document == nil {
		return DocumentSnapshot{}, false
	}
	return DocumentSnapshot{
		URI:        document.uri,
		LanguageID: document.languageID,
		Text:       document.text,
		Version:    document.version,
		References: len(document.references),
	}, true
}

func canonicalDocumentURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "file" || parsed.Path == "" {
		return "", fmt.Errorf("invalid file document URI: %q", raw)
	}
	parsed.Path = path.Clean(parsed.Path)
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

type documentPosition struct {
	Line      uint64 `json:"line"`
	Character uint64 `json:"character"`
}

type documentRange struct {
	Start documentPosition `json:"start"`
	End   documentPosition `json:"end"`
}

func applyContentChange(text string, raw json.RawMessage) (string, error) {
	var change struct {
		Range *documentRange `json:"range"`
		Text  string         `json:"text"`
	}
	if err := json.Unmarshal(raw, &change); err != nil {
		return "", fmt.Errorf("decode content change: %w", err)
	}
	if change.Range == nil {
		return change.Text, nil
	}
	start, err := documentOffset(text, change.Range.Start)
	if err != nil {
		return "", err
	}
	end, err := documentOffset(text, change.Range.End)
	if err != nil {
		return "", err
	}
	if end < start {
		return "", errors.New("content change range ends before it starts")
	}
	return text[:start] + change.Text + text[end:], nil
}

func documentOffset(text string, position documentPosition) (int, error) {
	lineStart := 0
	for line := uint64(0); line < position.Line; line++ {
		relative := findByte(text[lineStart:], '\n')
		if relative < 0 {
			return 0, errors.New("content change line exceeds document")
		}
		lineStart += relative + 1
	}
	lineEnd := len(text)
	if relative := findByte(text[lineStart:], '\n'); relative >= 0 {
		lineEnd = lineStart + relative
	}
	units := uint64(0)
	for offset := lineStart; offset < lineEnd; {
		if units == position.Character {
			return offset, nil
		}
		r, size := utf8.DecodeRuneInString(text[offset:lineEnd])
		width := uint64(len(utf16.Encode([]rune{r})))
		if units+width > position.Character {
			return 0, errors.New("content change splits UTF-16 surrogate pair")
		}
		units += width
		offset += size
	}
	if units == position.Character {
		return lineEnd, nil
	}
	return 0, errors.New("content change character exceeds line")
}

func findByte(text string, target byte) int {
	for index := 0; index < len(text); index++ {
		if text[index] == target {
			return index
		}
	}
	return -1
}

func parseDocumentSaveCapability(raw json.RawMessage) documentSaveCapability {
	var capabilities struct {
		TextDocumentSync json.RawMessage `json:"textDocumentSync"`
	}
	if json.Unmarshal(raw, &capabilities) != nil || len(capabilities.TextDocumentSync) == 0 {
		return documentSaveCapability{}
	}
	var syncOptions struct {
		Save json.RawMessage `json:"save"`
	}
	if json.Unmarshal(capabilities.TextDocumentSync, &syncOptions) != nil || len(syncOptions.Save) == 0 {
		return documentSaveCapability{}
	}
	var enabled bool
	if json.Unmarshal(syncOptions.Save, &enabled) == nil {
		return documentSaveCapability{enabled: enabled}
	}
	var saveOptions struct {
		IncludeText bool `json:"includeText"`
	}
	if json.Unmarshal(syncOptions.Save, &saveOptions) != nil {
		return documentSaveCapability{}
	}
	return documentSaveCapability{enabled: true, includeText: saveOptions.IncludeText}
}
