import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Window as HappyDOMWindow } from "happy-dom";
import { expect, vi } from "vitest";
import { useTranslation } from "react-i18next";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import Link from "@/components/routing/app-link";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type { WorkspaceState } from "@/lib/state/store";
import type { WorkspacePayload } from "@/lib/types/backend";
import { registerWorkspacesHandlers } from "@/lib/ws/handlers/workspaces";
import WorkspaceEditPage from "./[id]/page";
import type { WorkspaceUpdates } from "./workspace-edit-save";

export type Workspace = WorkspaceState["items"][number];
export const TARGET_ID = "settings-workspace";
export const OTHER_ID = "other-workspace";
export const CREATED_ID = "new-workspace";
export const SETTINGS_PATH = `/settings/workspace/${TARGET_ID}`;
// i18n-exempt: Synthetic accepted workspace name used only by the integration fixture.
export const ACCEPTED_NAME = "Accepted name";
// i18n-exempt: Synthetic unsaved workspace name used only by the integration fixture.
export const NEWER_NAME = "Newer unsaved name";

// i18n-exempt: Synthetic executor name used only by the integration fixture.
const EXECUTOR_NAME = "Local executor";
// i18n-exempt: Synthetic profile label used only by the integration fixture.
const PROFILE_LABEL = "Agent profile";
// i18n-exempt: Synthetic agent name used only by the integration fixture.
const AGENT_NAME = "Agent";

export function workspace(id: string, overrides: Partial<Workspace> = {}): Workspace {
  return {
    id,
    // i18n-exempt: Synthetic workspace names used only by the integration fixture.
    name: id === TARGET_ID ? "Original target" : "Original other",
    // i18n-exempt: Synthetic workspace metadata used only by the integration fixture.
    description: `Description ${id}`,
    owner_id: "original-owner",
    scopes: ["workspace.manage"],
    unit_id: "original-unit",
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: "original-config",
    acp_idle_suspension_enabled: false,
    acp_idle_timeout_minutes: 120,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-02T00:00:00Z",
    ...overrides,
  };
}

type Coordinator = ReturnType<typeof useSettingsSaveCoordinator>;
type Store = ReturnType<typeof useAppStoreApi>;
type SaveResult = Awaited<ReturnType<Coordinator["saveAll"]>>;
export type PatchTicket = {
  updates: WorkspaceUpdates;
  settled: boolean;
  finish: (value: Workspace, status?: number) => void;
};

export type Fixture = {
  store: Store;
  coordinator: Coordinator;
  tickets: PatchTicket[];
  saves: Promise<SaveResult>[];
  unexpected: string[];
  reads: string[];
  originalPath: string;
};
let fixture: Fixture | undefined;

function Observer({ current }: { current: Fixture }) {
  const { t } = useTranslation();
  current.store = useAppStoreApi();
  current.coordinator = useSettingsSaveCoordinator();
  return (
    <Link href="/settings/workspaces" data-testid="leave-settings">
      {t("common:back")}
    </Link>
  );
}

function json(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function transport(current: Fixture, input: RequestInfo | URL, init?: RequestInit) {
  const path = new URL(String(input), window.location.origin).pathname;
  const method = init?.method ?? "GET";
  if (path === `/api/v1/workspaces/${TARGET_ID}` && method === "PATCH") {
    let resolve!: (value: Response) => void;
    const promise = new Promise<Response>((done) => {
      resolve = done;
    });
    const ticket: PatchTicket = {
      updates: JSON.parse(String(init?.body)),
      settled: false,
      finish(value, status = 200) {
        if (ticket.settled) return;
        ticket.settled = true;
        resolve(status === 200 ? json(value) : json({ error: "Settings save rejected" }, status));
      },
    };
    current.tickets.push(ticket);
    return promise;
  }
  const readValues: Record<string, unknown> = {
    "/api/v1/units": { units: [], total: 0 },
    [`/api/v1/workspaces/${TARGET_ID}/members`]: { members: [] },
    "/api/v1/users/directory": { users: [] },
  };
  if (method === "GET" && path in readValues) {
    current.reads.push(path);
    return Promise.resolve(json(readValues[path]));
  }
  if (method === "POST" && path === "/api/v1/system/logs/frontend-errors") {
    return Promise.resolve(json({}));
  }
  current.unexpected.push(`${method} ${path}`);
  throw new Error(`Unexpected external transport: ${method} ${path}`);
}

export async function mountWorkspace(overrides: Partial<Workspace> = {}): Promise<Fixture> {
  const current = {
    tickets: [],
    saves: [],
    unexpected: [],
    reads: [],
    originalPath: window.location.pathname + window.location.search,
  } as unknown as Fixture;
  fixture = current;
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", SETTINGS_PATH);
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) =>
    transport(current, input, init),
  );
  render(
    <StateProvider
      initialState={{
        workspaces: {
          items: [workspace(TARGET_ID, overrides), workspace(OTHER_ID)],
          activeId: TARGET_ID,
          activeIdRevision: 7,
        },
        executors: {
          items: [
            {
              id: "local-executor",
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
          orderByAgent: {},
          version: 0,
          items: [
            {
              id: "agent-profile",
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
            <Observer current={current} />
            <WorkspaceEditPage workspaceId={TARGET_ID} />
            <AppSidebarWorkspacePicker modal={false} />
          </SettingsSaveProvider>
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  await waitFor(() => expect(current.reads).toHaveLength(3));
  await act(async () => {});
  expect(current.unexpected).toEqual([]);
  return current;
}

export function editName(name = ACCEPTED_NAME) {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: name } });
}

export async function beginSave(current: Fixture): Promise<PatchTicket> {
  act(() => {
    current.saves.push(current.coordinator.saveAll());
  });
  await waitFor(() => expect(current.tickets.at(-1)).toBeDefined());
  return current.tickets.at(-1)!;
}

export async function finishSave(
  current: Fixture,
  ticket: PatchTicket,
  value = workspace(TARGET_ID, { name: ACCEPTED_NAME }),
  status = 200,
) {
  let result!: SaveResult;
  await act(async () => {
    ticket.finish(value, status);
    result = await current.saves.at(-1)!;
  });
  expect(current.unexpected).toEqual([]);
  return result;
}

export function notify(
  current: Fixture,
  action: "workspace.created" | "workspace.updated" | "workspace.deleted",
  payload: WorkspacePayload,
) {
  act(() => {
    const handlers = registerWorkspacesHandlers(current.store);
    switch (action) {
      case "workspace.created":
        handlers["workspace.created"]!({ type: "notification", action, payload });
        break;
      case "workspace.updated":
        handlers["workspace.updated"]!({ type: "notification", action, payload });
        break;
      case "workspace.deleted":
        handlers["workspace.deleted"]!({ type: "notification", action, payload });
        break;
    }
  });
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

export function catalogue(current: Fixture) {
  return current.store.getState().workspaces.items;
}

export async function chooseDefault(index: number, label: string) {
  fireEvent.keyDown(screen.getAllByRole("combobox")[index], { key: "ArrowDown" });
  fireEvent.click(await screen.findByRole("option", { name: label }));
}

export async function assertGuardedRoute() {
  fireEvent.click(screen.getByTestId("leave-settings"));
  await screen.findByRole("alertdialog");
  expect(window.location.pathname).toBe(SETTINGS_PATH);
}

export async function cleanupWorkspaceFixture() {
  if (fixture) {
    const current = fixture;
    await act(async () => {
      for (const ticket of current.tickets) ticket.finish(workspace(TARGET_ID));
      await Promise.all(current.saves);
    });
    cleanup();
    clearNavigationBlockerForTests();
    window.history.replaceState({}, "", current.originalPath);
    expect(current.unexpected).toEqual([]);
  }
  fixture = undefined;
  await (window as unknown as HappyDOMWindow).happyDOM.abort();
  vi.unstubAllGlobals();
}
