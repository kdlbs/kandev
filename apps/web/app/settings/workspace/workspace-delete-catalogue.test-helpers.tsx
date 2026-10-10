import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Window as HappyDOMWindow } from "happy-dom";
import { expect, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import { registerWorkspacesHandlers } from "@/lib/ws/handlers/workspaces";
import type { WorkspaceState } from "@/lib/state/store";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import type { WorkspacePayload } from "@/lib/types/backend";
import WorkspaceEditPage from "./[id]/page";

export const TARGET = "delete-target";
export const SURVIVOR = "retained-workspace";
export const REMOVED = "independently-removed";
export const ADDED = "independently-added";
export const EDIT_PATH = `/settings/workspace/${TARGET}`;
// i18n-exempt: Synthetic workspace name used only by the integration fixture.
const TARGET_NAME = "Target workspace";
// i18n-exempt: Synthetic executor name used only by the integration fixture.
const EXECUTOR_NAME = "Local executor";
// i18n-exempt: Synthetic profile label used only by the integration fixture.
const PROFILE_LABEL = "Agent profile";
// i18n-exempt: Synthetic agent name used only by the integration fixture.
const AGENT_NAME = "Agent";
export type Workspace = WorkspaceState["items"][number];
type Store = ReturnType<typeof useAppStoreApi>;
type Ticket = { path: string; body: unknown; settled: boolean; finish: (status: number) => void };
export type Fixture = {
  store: Store;
  tickets: Ticket[];
  unexpected: string[];
  reads: string[];
  originalPath: string;
};
let mounted: Fixture | undefined;

export function row(id: string, overrides: Partial<Workspace> = {}): Workspace {
  return {
    id,
    // i18n-exempt: Synthetic workspace names used only by the integration fixture.
    name: id === TARGET ? TARGET_NAME : `Workspace ${id}`,
    // i18n-exempt: Synthetic workspace metadata used only by the integration fixture.
    description: `Original description ${id}`,
    owner_id: "owner-before",
    scopes: ["workspace.manage"],
    unit_id: "unit-before",
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: "config-before",
    acp_idle_suspension_enabled: false,
    acp_idle_timeout_minutes: 120,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-02T00:00:00Z",
    ...overrides,
  };
}

function StoreObserver({ fixture }: { fixture: Fixture }) {
  fixture.store = useAppStoreApi();
  return null;
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function serve(fixture: Fixture, input: RequestInfo | URL, init?: RequestInit) {
  const path = new URL(String(input), window.location.origin).pathname;
  const method = init?.method ?? "GET";
  if (
    method === "DELETE" &&
    (path === `/api/v1/workspaces/${TARGET}` || path === `/api/v1/office/workspaces/${TARGET}`)
  ) {
    return new Promise<Response>((resolve) => {
      const ticket: Ticket = {
        path,
        body: JSON.parse(String(init?.body)),
        settled: false,
        finish(status) {
          if (ticket.settled) return;
          ticket.settled = true;
          resolve(
            status === 204
              ? new Response(null, { status })
              : json({ error: "Delete refused" }, status),
          );
        },
      };
      fixture.tickets.push(ticket);
    });
  }
  const responses: Record<string, unknown> = {
    "/api/v1/units": { units: [], total: 0 },
    [`/api/v1/workspaces/${TARGET}/members`]: { members: [] },
    "/api/v1/users/directory": { users: [] },
  };
  if (method === "GET" && path in responses) {
    fixture.reads.push(path);
    return Promise.resolve(json(responses[path]));
  }
  if (method === "POST" && path === "/api/v1/system/logs/frontend-errors") {
    return Promise.resolve(json({}));
  }
  fixture.unexpected.push(`${method} ${path}`);
  throw new Error(`Unexpected transport: ${method} ${path}`);
}

export async function mountDelete({
  office = false,
  items = [row(TARGET), row(SURVIVOR), row(REMOVED)],
} = {}) {
  const fixture = {
    tickets: [],
    unexpected: [],
    reads: [],
    originalPath: window.location.pathname + window.location.search,
  } as unknown as Fixture;
  mounted = fixture;
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", EDIT_PATH);
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) =>
    serve(fixture, input, init),
  );
  render(
    <StateProvider
      initialState={{
        features: { ...defaultFeatureFlags, office },
        workspaces: { items, activeId: TARGET, activeIdRevision: 11 },
        executors: {
          items: [
            {
              id: "local",
              name: EXECUTOR_NAME,
              type: "local",
              status: "active",
              is_system: true,
              created_at: "",
              updated_at: "",
            },
          ],
        },
        agentProfiles: {
          version: 0,
          items: [
            {
              id: "profile",
              label: PROFILE_LABEL,
              agent_id: "agent",
              agent_name: AGENT_NAME,
              cli_passthrough: false,
            },
          ],
        },
      }}
    >
      <ToastProvider>
        <TooltipProvider>
          <SettingsSaveProvider>
            <StoreObserver fixture={fixture} />
            <WorkspaceEditPage workspaceId={TARGET} />
            <AppSidebarWorkspacePicker modal={false} />
          </SettingsSaveProvider>
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  await waitFor(() => expect(fixture.reads).toHaveLength(3));
  await act(async () => {});
  expect(fixture.unexpected).toEqual([]);
  return fixture;
}

export async function openDelete(name = TARGET_NAME) {
  fireEvent.click(screen.getByTestId("workspace-settings-delete-button"));
  const input = await screen.findByTestId("workspace-settings-delete-confirm-input");
  fireEvent.change(input, { target: { value: name } });
}

export async function beginDelete(fixture: Fixture) {
  await openDelete();
  fireEvent.click(screen.getByTestId("workspace-settings-delete-confirm-button"));
  await waitFor(() => expect(fixture.tickets).toHaveLength(1));
  expect(fixture.tickets[0].body).toEqual({ confirm_name: "Target workspace" });
  expect(window.location.pathname).toBe(EDIT_PATH);
  return fixture.tickets[0];
}

export async function finishDelete(fixture: Fixture, status = 204) {
  await act(async () => fixture.tickets[0].finish(status));
  if (status === 204)
    await waitFor(() => expect(window.location.pathname).toBe("/settings/workspaces"));
  else await screen.findByText("Delete refused");
  expect(fixture.unexpected).toEqual([]);
}

export function notify(
  fixture: Fixture,
  action: "workspace.created" | "workspace.updated" | "workspace.deleted",
  workspace: Workspace,
) {
  const payload: WorkspacePayload = {
    id: workspace.id,
    name: workspace.name,
    description: workspace.description ?? undefined,
    owner_id: workspace.owner_id,
    unit_id: workspace.unit_id,
    default_executor_id: workspace.default_executor_id,
    default_environment_id: workspace.default_environment_id,
    default_agent_profile_id: workspace.default_agent_profile_id,
    default_config_agent_profile_id: workspace.default_config_agent_profile_id,
    acp_idle_suspension_enabled: workspace.acp_idle_suspension_enabled,
    acp_idle_timeout_minutes: workspace.acp_idle_timeout_minutes,
    created_at: workspace.created_at,
    updated_at: workspace.updated_at,
  };
  act(() => {
    const handlers = registerWorkspacesHandlers(fixture.store);
    switch (action) {
      case "workspace.created":
        handlers[action]!({ type: "notification", action, payload });
        break;
      case "workspace.updated":
        handlers[action]!({ type: "notification", action, payload });
        break;
      case "workspace.deleted":
        handlers[action]!({ type: "notification", action, payload });
        break;
    }
  });
}

export function catalogue(fixture: Fixture) {
  return fixture.store.getState().workspaces.items;
}

export function select(fixture: Fixture, id: string | null) {
  act(() => fixture.store.getState().setActiveWorkspace(id));
  return fixture.store.getState().workspaces;
}

export async function openPicker() {
  const trigger = screen.getByTestId("sidebar-workspace-trigger");
  if (trigger.getAttribute("aria-expanded") !== "true")
    fireEvent.keyDown(trigger, { key: "Enter" });
  await screen.findByRole("menu");
}

export function choice(id: string) {
  return screen.queryByTestId(`sidebar-workspace-item-${id}`);
}

export async function cleanupDelete() {
  if (mounted) {
    const fixture = mounted;
    await act(async () => {
      for (const ticket of fixture.tickets) ticket.finish(503);
    });
    cleanup();
    clearNavigationBlockerForTests();
    window.history.replaceState({}, "", fixture.originalPath);
    expect(fixture.unexpected).toEqual([]);
  }
  mounted = undefined;
  await (window as unknown as HappyDOMWindow).happyDOM.abort();
  vi.unstubAllGlobals();
}
