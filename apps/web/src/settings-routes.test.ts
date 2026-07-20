import { QueryClient } from "@tanstack/react-query";
import { isValidElement, type ReactElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import IntegrationsGitLabPage from "@/app/settings/integrations/gitlab/page";
import { TaskActionsSettings } from "@/components/settings/general-settings";
import { qk } from "@/lib/query/keys";
import { workspaceId, workflowId } from "@/lib/types/ids";
import type { ListWorkspacesResponse, UserSettingsResponse } from "@/lib/types/http";
import {
  applySettingsInitialState,
  buildSettingsInitialStateForRoute,
  renderSettingsRoute,
} from "./settings-routes";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
const OWNER_ID = "owner-1";
const TIMESTAMP = "2026-01-01T00:00:00Z";

describe("buildSettingsInitialStateForRoute", () => {
  beforeEach(() => {
    document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=; path=/; max-age=0`;
  });

  describe("workspace selection", () => {
    it("keeps the saved active workspace for settings hydration", () => {
      const state = buildState({
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-1") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-1");
      expect(state.userSettings?.workspaceId).toBe("ws-1");
    });

    it("keeps the active workspace cookie on global settings pages", () => {
      document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=ws-2; path=/`;

      const state = buildState({
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-1") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-2");
      expect(state.userSettings?.workspaceId).toBe("ws-2");
    });

    it("uses the route workspace on workspace settings pages", () => {
      document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=ws-1; path=/`;

      const state = buildState({
        pathname: "/settings/workspace/ws-2/integrations/gitlab",
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-1") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-2");
      expect(state.userSettings?.workspaceId).toBe("ws-2");
    });

    it("falls back to user settings when cookie has an office workspace", () => {
      document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=ws-office; path=/`;

      const state = buildState({
        workspaces: [
          buildWorkspace({ id: "ws-office", office_workflow_id: workflowId("office") }),
          buildWorkspace({ id: "ws-kanban", office_workflow_id: null }),
        ],
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-kanban") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-kanban");
      expect(state.userSettings?.workspaceId).toBe("ws-kanban");
    });
  });

  describe("fallbacks", () => {
    it("falls back to the settings workspace_id when no cookie matches", () => {
      const state = buildState({
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-2") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-2");
      expect(state.userSettings?.workspaceId).toBe("ws-2");
    });

    it("falls back to the first workspace when neither cookie nor settings match", () => {
      const state = buildState({
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("missing") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-1");
      expect(state.userSettings?.workspaceId).toBe("ws-1");
    });

    it("returns empty state defaults when all API calls fail", () => {
      const state = buildState({ userSettingsResponse: null });

      expect(state.workspaces).toEqual({ items: [], activeId: null });
      expect(state.executors).toEqual({ items: [] });
      expect(state.settingsAgents).toEqual({ items: [] });
      expect(state.agentDiscovery).toEqual({ items: [], loading: false, loaded: true });
      expect(state.availableAgents).toEqual({
        items: [],
        tools: [],
        loading: false,
        loaded: true,
      });
      expect(state.userSettings).toBeUndefined();
    });
  });

  it("only spreads userSettings when settings were loaded", () => {
    const loaded = buildState({
      workspaces: workspaceRows(["ws-1"]),
      userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-1") }),
    });
    const failed = buildState({
      workspaces: workspaceRows(["ws-1"]),
      userSettingsResponse: null,
    });

    expect(loaded.userSettings?.loaded).toBe(true);
    expect(failed.userSettings).toBeUndefined();
  });
});

describe("renderSettingsRoute", () => {
  it("renders layout profile settings from General settings", () => {
    const route = renderSettingsRoute("/settings/general/layouts");
    expect(isValidElement(route)).toBe(true);
    expect(((route as ReactElement).type as { name?: string }).name).toBe("LayoutSettings");
  });

  it("renders task action preferences from General settings", () => {
    const route = renderSettingsRoute("/settings/general/task-actions");
    expect(isValidElement(route)).toBe(true);
    expect((route as ReactElement).type).toBe(TaskActionsSettings);
  });

  it("passes the route workspace id to the GitLab integration page", () => {
    expect(gitLabRouteWorkspaceId("/settings/workspace/ws-2/integrations/gitlab")).toBe("ws-2");
    expect(gitLabRouteWorkspaceId("/settings/workspace/ws%202/integrations/gitlab")).toBe("ws 2");
  });
});

describe("applySettingsInitialState", () => {
  it("hydrates the root store and seeds settings query keys", () => {
    const queryClient = new QueryClient();
    const hydrate = vi.fn();
    const state = buildState({
      executors: [{ id: "executor-1", name: "Docker" }],
      agents: [
        {
          id: "agent-1",
          name: "codex",
          profiles: [{ id: "profile-1", agentDisplayName: "Codex", name: "Default" }],
        },
      ],
    } as unknown as Partial<Parameters<typeof buildSettingsInitialStateForRoute>[0]>);

    applySettingsInitialState({ getState: () => ({ hydrate }) }, queryClient, state);

    expect(hydrate).toHaveBeenCalledWith(state);
    expect(queryClient.getQueryData(qk.settings.executors())).toEqual({
      executors: [{ id: "executor-1", name: "Docker" }],
    });
    expect(queryClient.getQueryData(qk.settings.agents())).toEqual({
      agents: state.settingsAgents?.items,
      total: 1,
    });
  });
});

function buildState(
  overrides: Partial<Parameters<typeof buildSettingsInitialStateForRoute>[0]> = {},
) {
  return buildSettingsInitialStateForRoute({
    pathname: "/settings",
    workspaces: [],
    executors: [],
    agents: [],
    discoveryAgents: [],
    availableAgents: [],
    availableTools: [],
    userSettingsResponse: null,
    ...overrides,
  });
}

function buildWorkspace(
  params: Omit<
    Partial<ListWorkspacesResponse["workspaces"][number]>,
    "id" | "office_workflow_id"
  > & {
    id: string;
    office_workflow_id: ReturnType<typeof workflowId> | null;
  },
) {
  const { id, office_workflow_id, ...rest } = params;
  return {
    id: workspaceId(id),
    name: `Workspace ${id}`,
    description: null,
    owner_id: OWNER_ID,
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: null,
    office_workflow_id,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...rest,
  } as unknown as ListWorkspacesResponse["workspaces"][number];
}

function workspaceRows(ids: string[]): ListWorkspacesResponse["workspaces"] {
  return ids.map((id) => buildWorkspace({ id, office_workflow_id: null }));
}

function userSettings(
  settings: Partial<NonNullable<UserSettingsResponse["settings"]>>,
): UserSettingsResponse {
  return {
    settings: {
      user_id: OWNER_ID,
      workspace_id: workspaceId(""),
      workflow_filter_id: workflowId(""),
      repository_ids: [],
      updated_at: TIMESTAMP,
      ...settings,
    },
  };
}

function gitLabRouteWorkspaceId(pathname: string): string | undefined {
  const route = renderSettingsRoute(pathname);
  if (!isValidElement(route)) {
    throw new Error("expected GitLab integration route element");
  }
  expect(route.type).toBe(IntegrationsGitLabPage);
  return (route as ReactElement<{ workspaceId?: string }>).props.workspaceId;
}
