// Package pluginsdk is the author-facing SDK for kandev plugin backends, and
// the shared wiring kandev's own runtime manager imports to spawn and talk
// to those plugins. See docs/plans/plugins/GRPC-CONTRACT.md §4 for the
// frozen public surface this package implements.
//
// types.go defines the Go-native mirrors of the kandev.plugin.v1 proto
// messages (using map[string]any in place of google.protobuf.Struct) plus
// the proto<->Go conversion helpers used by every other file in this
// package. Authors and kandev's runtime manager only ever see these
// Go-native types; proto types from
// github.com/kandev/kandev/proto/kandev/plugin/v1 never leak past the
// package boundary.
package pluginsdk

import (
	"fmt"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// Event is the Go-native mirror of kandev.plugin.v1.Event, delivered to a
// Plugin's OnEvent method.
type Event struct {
	EventID     string
	EventType   string
	OccurredAt  string
	WorkspaceID string
	Payload     map[string]any
}

func (e *Event) toProto() (*pluginv1.Event, error) {
	payload, err := mapToStruct(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: event payload: %w", err)
	}
	return &pluginv1.Event{
		EventId:     e.EventID,
		EventType:   e.EventType,
		OccurredAt:  e.OccurredAt,
		WorkspaceId: e.WorkspaceID,
		Payload:     payload,
	}, nil
}

func eventFromProto(p *pluginv1.Event) (*Event, error) {
	payload, err := structToMap(p.GetPayload())
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: event payload: %w", err)
	}
	return &Event{
		EventID:     p.GetEventId(),
		EventType:   p.GetEventType(),
		OccurredAt:  p.GetOccurredAt(),
		WorkspaceID: p.GetWorkspaceId(),
		Payload:     payload,
	}, nil
}

// WebhookRequest is the Go-native mirror of kandev.plugin.v1.WebhookRequest,
// delivered to a Plugin's HandleWebhook method. Unlike Event, it has no
// Struct-typed fields so conversion cannot fail.
type WebhookRequest struct {
	WebhookKey string
	Method     string
	Path       string
	Query      string
	Headers    map[string]string
	Body       []byte
}

func (r *WebhookRequest) toProto() *pluginv1.WebhookRequest {
	return &pluginv1.WebhookRequest{
		WebhookKey: r.WebhookKey,
		Method:     r.Method,
		Path:       r.Path,
		Query:      r.Query,
		Headers:    r.Headers,
		Body:       r.Body,
	}
}

func webhookRequestFromProto(p *pluginv1.WebhookRequest) *WebhookRequest {
	return &WebhookRequest{
		WebhookKey: p.GetWebhookKey(),
		Method:     p.GetMethod(),
		Path:       p.GetPath(),
		Query:      p.GetQuery(),
		Headers:    p.GetHeaders(),
		Body:       p.GetBody(),
	}
}

// WebhookResponse is the Go-native mirror of kandev.plugin.v1.WebhookResponse,
// returned by a Plugin's HandleWebhook method.
type WebhookResponse struct {
	Status  int32
	Headers map[string]string
	Body    []byte
}

func (r *WebhookResponse) toProto() *pluginv1.WebhookResponse {
	return &pluginv1.WebhookResponse{
		Status:  r.Status,
		Headers: r.Headers,
		Body:    r.Body,
	}
}

func webhookResponseFromProto(p *pluginv1.WebhookResponse) *WebhookResponse {
	return &WebhookResponse{
		Status:  p.GetStatus(),
		Headers: p.GetHeaders(),
		Body:    p.GetBody(),
	}
}

// StateEntry is the Go-native mirror of kandev.plugin.v1.StateEntry, as
// returned by Host.ListState.
type StateEntry struct {
	Key       string
	Value     map[string]any
	UpdatedAt string
}

func (e *StateEntry) toProto() (*pluginv1.StateEntry, error) {
	value, err := mapToStruct(e.Value)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: state entry value: %w", err)
	}
	return &pluginv1.StateEntry{
		Key:       e.Key,
		Value:     value,
		UpdatedAt: e.UpdatedAt,
	}, nil
}

func stateEntryFromProto(p *pluginv1.StateEntry) (*StateEntry, error) {
	value, err := structToMap(p.GetValue())
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: state entry value: %w", err)
	}
	return &StateEntry{
		Key:       p.GetKey(),
		Value:     value,
		UpdatedAt: p.GetUpdatedAt(),
	}, nil
}

func stateEntriesFromProto(entries []*pluginv1.StateEntry) ([]StateEntry, error) {
	if entries == nil {
		return nil, nil
	}
	out := make([]StateEntry, len(entries))
	for i, e := range entries {
		converted, err := stateEntryFromProto(e)
		if err != nil {
			return nil, err
		}
		out[i] = *converted
	}
	return out, nil
}

// mapToStruct converts a Go-native map to a google.protobuf.Struct. A nil
// map converts to a nil Struct (distinguishing "no payload" from "empty
// payload" across the wire).
func mapToStruct(m map[string]any) (*structpb.Struct, error) {
	if m == nil {
		return nil, nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: convert map to struct: %w", err)
	}
	return s, nil
}

// structToMap converts a google.protobuf.Struct to a Go-native map. A nil
// Struct converts to a nil map.
func structToMap(s *structpb.Struct) (map[string]any, error) {
	if s == nil {
		return nil, nil
	}
	return s.AsMap(), nil
}
