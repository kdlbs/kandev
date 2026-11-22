package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type savedDisclosureResolver struct{}

func (savedDisclosureResolver) ResolveSidebarGrouping(context.Context, string, string) (*models.SidebarPluginGrouping, error) {
	return &models.SidebarPluginGrouping{Reference: "plugin:acme:labels", Generation: "g", Snapshot: &pluginsdk.SidebarGroupingSnapshot{
		Version: 1, Complete: true, Revision: "r", Groups: []pluginsdk.SidebarGroupingValue{{ID: "keep", Label: "Keep"}, {ID: "reopen", Label: "Reopen"}},
	}}, nil
}

func TestSidebarSavedDisclosureIntersectsTheCompleteProviderSnapshot(t *testing.T) {
	reference := "plugin:acme:labels"
	keep := fmt.Sprintf("plugin-group:%x", sha256.Sum256([]byte(reference+"\x00keep")))
	keys := make([]string, 6000, 6002)
	for i := range keys {
		keys[i] = fmt.Sprintf("plugin-group:%064x", i)
	}
	keys = append(keys, keep, reference+":untagged")
	repo := &httpTaskRepo{sidebarPage: &models.SidebarTaskPageResult{Page: 1, PageSize: 100}}
	h := newHTTPTaskHandlers(t, repo)
	h.service.SetSidebarGroupingResolver(savedDisclosureResolver{})
	settings := &usermodels.UserSettings{SidebarViewsByWorkspace: map[string]usermodels.SidebarWorkspaceState{
		"ws-b": {Views: []usermodels.SidebarView{{ID: "saved", Name: "Saved", Group: reference, CollapsedGroups: keys}}},
	}}
	h.SetSidebarTaskSettingsReader(sidebarSettingsReader{settings: settings})
	read := func(id string) int {
		c, rec := taskRequestAs(t, "", http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", "ws-b")
		c.Request.Body = io.NopCloser(strings.NewReader(`{"group":"plugin:acme:labels","disclosure_view_id":"` + id + `"}`))
		h.httpQuerySidebarTasks(c)
		return rec.Code
	}
	require.Equal(t, http.StatusOK, read("saved"))
	require.Equal(t, []string{keep, reference + ":unassigned"}, repo.sidebarQuery.CollapsedGroupKeys)
	state := settings.SidebarViewsByWorkspace["ws-b"]
	state.Views[0].CollapsedGroups = keys[:6000]
	require.Equal(t, http.StatusOK, read("saved"))
	require.Empty(t, repo.sidebarQuery.CollapsedGroupKeys, "reopening must not revive retired keys")
	require.Equal(t, http.StatusBadRequest, read("another-users-view"))
}

func TestSidebarSavedDraftDisclosureUsesItsOwnGrouping(t *testing.T) {
	reference := "plugin:acme:labels"
	keep := fmt.Sprintf("plugin-group:%x", sha256.Sum256([]byte(reference+"\x00keep")))
	var state usermodels.SidebarWorkspaceState
	require.NoError(t, json.Unmarshal([]byte(`{"views":[{"id":"saved","name":"Tag","group":"plugin:acme:other","collapsed_groups":["base-only"]}],"active_view_id":"saved","draft":{"base_view_id":"saved","group":"plugin:acme:labels","collapsed_groups":["`+keep+`"]}}`), &state))
	repo := &httpTaskRepo{sidebarPage: &models.SidebarTaskPageResult{Page: 1, PageSize: 100}}
	h := newHTTPTaskHandlers(t, repo)
	h.service.SetSidebarGroupingResolver(savedDisclosureResolver{})
	h.SetSidebarTaskSettingsReader(sidebarSettingsReader{settings: &usermodels.UserSettings{
		SidebarViewsByWorkspace: map[string]usermodels.SidebarWorkspaceState{"ws-b": state},
	}})
	c, rec := taskRequestAs(t, "", http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", "ws-b")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"group":"plugin:acme:labels","disclosure_view_id":"saved"}`))
	h.httpQuerySidebarTasks(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{keep}, repo.sidebarQuery.CollapsedGroupKeys)
	require.Equal(t, []string{"base-only"}, state.Views[0].CollapsedGroups)
}
