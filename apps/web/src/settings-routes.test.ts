import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ReactElement } from "react";
import { render, screen } from "@testing-library/react";

import ExecutorEditPage from "@/app/settings/executor/[id]/page";
import ProfileDetailPage from "@/app/settings/executor/[id]/profile/[profileId]/page";
import ExecutorCreatePage from "@/app/settings/executor/new/page";
import ProfileEditPage from "@/app/settings/executors/[profileId]/page";
import CreateProfilePage from "@/app/settings/executors/new/[type]/page";
import SSHExecutorPage from "@/app/settings/executors/ssh/[executorId]/page";
import IntegrationsGitHubPage from "@/app/settings/integrations/github/page";
import IntegrationsGitLabPage from "@/app/settings/integrations/gitlab/page";
import PluginDetailPage from "@/app/settings/plugins/[pluginId]/page";
import AutomationEditorPage from "@/app/settings/workspace/[id]/automations/[automationId]/page";
import NewAutomationPage from "@/app/settings/workspace/[id]/automations/new/page";
import WorkspaceEditPage from "@/app/settings/workspace/[id]/page";
import { TaskBehaviorSettings } from "@/components/settings/task-behavior-settings";
import { WorkspaceSettingsShell } from "@/components/settings/workspaces/workspace-settings-shell";
import { workspaceId, workflowId } from "@/lib/types/ids";
import type { ListWorkspacesResponse, UserSettingsResponse } from "@/lib/types/http";
import { DEFAULT_SETTINGS_PATH } from "@/lib/settings/last-settings-page";
import {
  buildSettingsInitialStateForRoute,
  renderSettingsRoute,
  SETTINGS_ROUTE_PATHS,
} from "./settings-routes";

vi.mock("@/components/settings/system/updates-card", () => ({ UpdatesCard: () => null }));

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
      expect(state.agentProfiles).toEqual({ items: [], version: 0 });
      expect(state.settingsAgents).toEqual({ items: [] });
      expect(state.agentDiscovery).toEqual({ items: [], loading: false, loaded: true });
      expect(state.availableAgents).toEqual({
        items: [],
        tools: [],
        loading: false,
        loaded: true,
      });
      expect(state.settingsData).toEqual({ executorsLoaded: true, agentsLoaded: true });
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

  it("does not recreate the retired update-notification state during hydration", () => {
    const state = buildState();

    expect(state.system).toBeUndefined();
  });
});

describe("message queue settings route", () => {
  it("renders the Message Queue inside the merged Task behavior page", () => {
    const route = renderSettingsRoute("/settings/preferences/task-behavior");

    expect(isValidElement(route)).toBe(true);
    expect((route as ReactElement).type).toBe(TaskBehaviorSettings);
  });

  it("redirects the former General URL to the Task behavior page", () => {
    const route = renderSettingsRoute("/settings/general/message-queue") as ReactElement<{
      to: string;
    }>;

    expect(isValidElement(route)).toBe(true);
    expect((route.type as { name?: string }).name).toBe("SettingsRedirect");
    expect(route.props.to).toBe("/settings/preferences/task-behavior");
  });

  it("redirects the former System URL to the Task behavior page", () => {
    const route = renderSettingsRoute("/settings/system/message-queue") as ReactElement<{
      to: string;
    }>;

    expect(isValidElement(route)).toBe(true);
    expect((route.type as { name?: string }).name).toBe("SettingsRedirect");
    expect(route.props.to).toBe("/settings/preferences/task-behavior");
  });
});

describe("renderSettingsRoute", () => {
  it("directs update notification settings to Notifications", () => {
    render(renderSettingsRoute("/settings/system/updates"));

    expect(screen.getByText(/notification preferences are managed/i)).toBeTruthy();
    expect(screen.getByRole("link", { name: /notifications/i }).getAttribute("href")).toBe(
      "/settings/preferences/notifications",
    );
  });

  it("renders layout profile settings from Preferences", () => {
    const route = renderSettingsRoute("/settings/preferences/layouts");
    expect(isValidElement(route)).toBe(true);
    expect(((route as ReactElement).type as { name?: string }).name).toBe("LayoutSettings");
  });

  it("redirects the legacy task actions page into Task behavior", () => {
    const route = renderSettingsRoute("/settings/general/task-actions") as ReactElement<{
      to: string;
    }>;
    expect(isValidElement(route)).toBe(true);
    expect((route.type as { name?: string }).name).toBe("SettingsRedirect");
    expect(route.props.to).toBe("/settings/preferences/task-behavior");
  });

  it("passes the route workspace id to the GitLab integration page", () => {
    expect(gitLabRouteWorkspaceId("/settings/workspaces/ws-2/integrations/gitlab")).toBe("ws-2");
    expect(gitLabRouteWorkspaceId("/settings/workspaces/ws%202/integrations/gitlab")).toBe("ws 2");
  });

  it("redirects legacy /settings/workspace/<id> paths into /settings/workspaces", () => {
    const route = renderSettingsRoute(
      "/settings/workspace/ws-1/integrations/github",
    ) as ReactElement<{ to: string }>;

    expect(isValidElement(route)).toBe(true);
    expect((route.type as { name?: string }).name).toBe("SettingsRedirect");
    expect(route.props.to).toBe("/settings/workspaces/ws-1/integrations/github");
  });

  it("routes GitHub settings through the integration page wrapper", () => {
    const route = renderSettingsRoute("/settings/integrations/github");

    expect(isValidElement(route)).toBe(true);
    expect((route as ReactElement).type).toBe(IntegrationsGitHubPage);
  });

  it.each([
    {
      pathname: "/settings/plugins/plugin%20one",
      component: PluginDetailPage,
      identifiers: { pluginId: "plugin one" },
    },
    {
      pathname: "/settings/executor/executor%20one/profile/profile%20one",
      component: ProfileDetailPage,
      identifiers: { executorId: "executor one", profileId: "profile one" },
    },
    {
      pathname: "/settings/executor/executor%20one",
      component: ExecutorEditPage,
      identifiers: { executorId: "executor one" },
    },
    {
      pathname: "/settings/executors/profile%20one",
      component: ProfileEditPage,
      identifiers: { profileId: "profile one" },
    },
    {
      pathname: "/settings/executors/new/local_docker",
      component: CreateProfilePage,
      identifiers: { executorType: "local_docker" },
    },
    {
      pathname: "/settings/executors/ssh/executor%20one",
      component: SSHExecutorPage,
      identifiers: { executorId: "executor one" },
    },
    {
      pathname: "/settings/workspaces/workspace%20one",
      component: WorkspaceEditPage,
      identifiers: { workspaceId: "workspace one" },
    },
    {
      pathname: "/settings/workspaces/workspace%20one/automations/new",
      component: NewAutomationPage,
      identifiers: { workspaceId: "workspace one" },
    },
    {
      pathname: "/settings/workspaces/workspace%20one/automations/automation%20one",
      component: AutomationEditorPage,
      identifiers: { workspaceId: "workspace one", automationId: "automation one" },
    },
  ])(
    "passes decoded synchronous identifiers for $pathname",
    ({ pathname, component, identifiers }) => {
      const route = unwrapWorkspaceShell(renderSettingsRoute(pathname));
      if (!isValidElement<Record<string, unknown>>(route)) {
        throw new Error(`expected a route element for ${pathname}`);
      }

      expect({
        component: route.type,
        identifiers: pickProps(route.props, Object.keys(identifiers)),
        thenableProps: Object.entries(route.props)
          .filter(([, value]) => isThenable(value))
          .map(([name]) => name),
        asyncComponent:
          typeof route.type === "function" && route.type.constructor.name === "AsyncFunction",
      }).toEqual({
        component,
        identifiers,
        thenableProps: [],
        asyncComponent: false,
      });
    },
  );

  it("reserves /settings/executor/new for executor creation", () => {
    const route = renderSettingsRoute("/settings/executor/new");

    expect(isValidElement(route)).toBe(true);
    expect((route as ReactElement).type).toBe(ExecutorCreatePage);
  });

  it("reserves /settings/agents/browse for the install catalogue, not an agent named browse", () => {
    const route = renderSettingsRoute("/settings/agents/browse");

    expect(isValidElement(route)).toBe(true);
    expect(((route as ReactElement).type as { name?: string }).name).toBe("AgentsBrowsePage");
  });
});

function buildState(
  overrides: Partial<Parameters<typeof buildSettingsInitialStateForRoute>[0]> = {},
) {
  return buildSettingsInitialStateForRoute({
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
  const route = unwrapWorkspaceShell(renderSettingsRoute(pathname));
  if (!isValidElement(route)) {
    throw new Error("expected GitLab integration route element");
  }
  expect(route.type).toBe(IntegrationsGitLabPage);
  return (route as ReactElement<{ workspaceId?: string }>).props.workspaceId;
}

// Workspace routes render inside the tabbed WorkspaceSettingsShell; unwrap it
// so identifier assertions land on the page component itself.
function unwrapWorkspaceShell(route: unknown): unknown {
  if (isValidElement(route) && route.type === WorkspaceSettingsShell) {
    return (route as ReactElement<{ children?: unknown }>).props.children;
  }
  return route;
}

function pickProps(props: Record<string, unknown>, names: string[]): Record<string, unknown> {
  return Object.fromEntries(names.map((name) => [name, props[name]]));
}

function isThenable(value: unknown): value is PromiseLike<unknown> {
  return (
    (typeof value === "object" || typeof value === "function") &&
    value !== null &&
    "then" in value &&
    typeof value.then === "function"
  );
}

describe("restorable settings paths", () => {
  it("covers the default /settings target and excludes what cannot be restored", () => {
    // `/settings` restores only into this set. It has to contain the fallback
    // target, and must not contain the shapes that resolve against deletable
    // records — those are matched dynamically, so membership is the guard.
    expect(SETTINGS_ROUTE_PATHS.has(DEFAULT_SETTINGS_PATH)).toBe(true);
    expect(DEFAULT_SETTINGS_PATH).toBe("/settings/preferences/appearance");

    for (const path of SETTINGS_ROUTE_PATHS) {
      expect(path.startsWith("/settings"), `${path} is not a settings path`).toBe(true);
      expect(path, `${path} looks like a dynamic route`).not.toMatch(/[[\]:*]/);
    }
    expect(SETTINGS_ROUTE_PATHS.has("/settings/plugins/kandev-plugin-e2e")).toBe(false);
    expect(SETTINGS_ROUTE_PATHS.has("/settings/does-not-exist")).toBe(false);
  });
});
