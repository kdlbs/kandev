package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

type webAppStateEntry struct {
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	Revision   int64           `json:"revision"`
	WriterKind string          `json:"writer_kind"`
	UpdatedAt  string          `json:"updated_at"`
}

func (s *Service) handleWebAppState(ctx context.Context, w http.ResponseWriter, r *http.Request, binding webapp.CapabilityBinding, parts []string) {
	if !webAppCapabilities(binding.Permissions).State {
		writeWebAppError(w, http.StatusForbidden, "plugin_permission_denied")
		return
	}
	store := s.InstanceState()
	if store == nil {
		writeWebAppError(w, http.StatusNotImplemented, webAppRuntimeUnavailable)
		return
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		s.listWebAppState(ctx, w, r, store, binding.InstanceID)
	case len(parts) == 3 && validWebAppStateKey(parts[2]) && r.Method == http.MethodGet:
		s.getWebAppState(ctx, w, r, store, binding.InstanceID, parts[2])
	case len(parts) == 3 && validWebAppStateKey(parts[2]) && r.Method == http.MethodPut:
		s.setWebAppState(ctx, w, r, store, binding.InstanceID, parts[2])
	case len(parts) == 3 && validWebAppStateKey(parts[2]) && r.Method == http.MethodDelete:
		s.deleteWebAppState(ctx, w, r, store, binding.InstanceID, parts[2])
	default:
		writeWebAppError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func validWebAppStateKey(key string) bool {
	return validWebAppKey(key) && !strings.HasPrefix(key, "_")
}

func (s *Service) listWebAppState(ctx context.Context, w http.ResponseWriter, r *http.Request, store *state.InstanceStore, instanceID string) {
	entries, err := store.List(ctx, instanceID)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	items := make([]webAppStateEntry, len(entries))
	for i, entry := range entries {
		items[i] = webAppStateEntryFromStore(entry)
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppPage[webAppStateEntry]{Items: items, PageInfo: webAppPageInfo{}})
}

func (s *Service) getWebAppState(ctx context.Context, w http.ResponseWriter, r *http.Request, store *state.InstanceStore, instanceID, key string) {
	entry, found, err := store.Get(ctx, instanceID, key)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	if !found {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppStateEntryFromStore(entry))
}

func (s *Service) setWebAppState(ctx context.Context, w http.ResponseWriter, r *http.Request, store *state.InstanceStore, instanceID, key string) {
	expected, ok := webAppIfMatch(r)
	if !ok {
		writeWebAppError(w, http.StatusPreconditionRequired, "plugin_state_precondition_required")
		return
	}
	body, err := readWebAppBody(r)
	if err != nil || !json.Valid(body) {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	entry, err := store.Set(ctx, instanceID, key, json.RawMessage(body), &expected, "browser")
	if err != nil {
		writeWebAppStateError(w, err)
		return
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppStateEntryFromStore(entry))
}

func (s *Service) deleteWebAppState(ctx context.Context, w http.ResponseWriter, r *http.Request, store *state.InstanceStore, instanceID, key string) {
	expected, ok := webAppIfMatch(r)
	if !ok {
		writeWebAppError(w, http.StatusPreconditionRequired, "plugin_state_precondition_required")
		return
	}
	revision, err := store.Delete(ctx, instanceID, key, &expected, "browser")
	if err != nil {
		writeWebAppStateError(w, err)
		return
	}
	writeWebAppJSON(w, r, http.StatusOK, struct {
		Revision int64 `json:"revision"`
	}{Revision: revision})
}

func webAppStateEntryFromStore(entry state.InstanceStateEntry) webAppStateEntry {
	return webAppStateEntry{
		Key: entry.Key, Value: append(json.RawMessage(nil), entry.Value...),
		Revision: entry.Revision, WriterKind: entry.WriterKind,
		UpdatedAt: entry.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func webAppIfMatch(r *http.Request) (int64, bool) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	if value == "" || strings.ContainsAny(value, "\"*, ") {
		return 0, false
	}
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 0 {
		return 0, false
	}
	return revision, true
}

func writeWebAppStateError(w http.ResponseWriter, err error) {
	var conflict *state.ConflictError
	if errors.As(err, &conflict) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write(mustMarshalWebAppStateConflict(conflict.CurrentRevision))
		return
	}
	writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
}

func mustMarshalWebAppStateConflict(revision int64) []byte {
	body, err := json.Marshal(struct {
		Error           string `json:"error"`
		CurrentRevision int64  `json:"current_revision"`
	}{Error: "plugin_state_conflict", CurrentRevision: revision})
	if err != nil {
		return []byte(`{"error":"plugin_state_conflict"}`)
	}
	return body
}

func readWebAppBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("request body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, webAppRequestLimit+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > webAppRequestLimit {
		return nil, errors.New("request body exceeds limit")
	}
	return body, nil
}

func decodeWebAppJSON(r *http.Request, destination any) error {
	body, err := readWebAppBody(r)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
