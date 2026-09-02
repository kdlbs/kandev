package lsp

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

const progressKindEnd = "end"

type progressParams struct {
	Token json.RawMessage `json:"token"`
	Value struct {
		Kind       string   `json:"kind"`
		Title      string   `json:"title"`
		Message    string   `json:"message"`
		Percentage *float64 `json:"percentage"`
	} `json:"value"`
}

func (m *Manager) applyProgress(taskID, language string, generation uint64, raw json.RawMessage) {
	var params progressParams
	if json.Unmarshal(raw, &params) != nil {
		return
	}
	token, ok := progressToken(params.Token)
	if !ok {
		return
	}
	m.publishForTaskGeneration(taskID, language, generation, func(snapshot *Snapshot) {
		index := workIndex(snapshot.Work, token)
		switch params.Value.Kind {
		case "begin":
			item := WorkItem{
				Token:      token,
				Title:      params.Value.Title,
				Message:    params.Value.Message,
				Percentage: normalizedPercentage(params.Value.Percentage),
				StartedAt:  time.Now().UTC(),
			}
			if index >= 0 {
				item.StartedAt = snapshot.Work[index].StartedAt
				snapshot.Work[index] = item
			} else {
				snapshot.Work = append(snapshot.Work, item)
			}
		case "report":
			if index < 0 {
				return
			}
			if params.Value.Message != "" {
				snapshot.Work[index].Message = params.Value.Message
			}
			if params.Value.Percentage != nil {
				snapshot.Work[index].Percentage = normalizedPercentage(params.Value.Percentage)
			}
		case progressKindEnd:
			if index >= 0 {
				item := snapshot.Work[index]
				message := params.Value.Message
				if message == "" {
					message = item.Message
				}
				snapshot.LastCompletedWork = &CompletedWorkItem{
					Token: item.Token, Title: item.Title, Message: message,
					StartedAt: item.StartedAt, CompletedAt: time.Now().UTC(),
				}
				snapshot.Work = append(snapshot.Work[:index], snapshot.Work[index+1:]...)
			}
		default:
			return
		}
		sortWork(snapshot.Work)
		if len(snapshot.Work) == 0 {
			snapshot.Activity = ActivityIdle
		} else {
			snapshot.Activity = ActivityServerWork
		}
	})
}

func (m *Manager) applyDiagnostics(taskID, language string, generation uint64, raw json.RawMessage) {
	var params struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(raw, &params) != nil || params.URI == "" {
		return
	}
	m.publishForTaskGeneration(taskID, language, generation, func(snapshot *Snapshot) {
		byURI := make(map[string]json.RawMessage, len(snapshot.Diagnostics)+1)
		for _, diagnostic := range snapshot.Diagnostics {
			var existing struct {
				URI string `json:"uri"`
			}
			if json.Unmarshal(diagnostic, &existing) == nil && existing.URI != "" {
				byURI[existing.URI] = diagnostic
			}
		}
		byURI[params.URI] = append(json.RawMessage(nil), raw...)
		uris := make([]string, 0, len(byURI))
		for uri := range byURI {
			uris = append(uris, uri)
		}
		sort.Strings(uris)
		snapshot.Diagnostics = make([]json.RawMessage, 0, len(uris))
		for _, uri := range uris {
			snapshot.Diagnostics = append(snapshot.Diagnostics, byURI[uri])
		}
	})
}

func progressToken(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, text != ""
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return strconv.FormatFloat(number, 'g', -1, 64), true
	}
	return "", false
}

func normalizedPercentage(value *float64) *float64 {
	if value == nil {
		return nil
	}
	normalized := *value
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 100 {
		normalized = 100
	}
	return &normalized
}

func workIndex(items []WorkItem, token string) int {
	for index := range items {
		if items[index].Token == token {
			return index
		}
	}
	return -1
}
