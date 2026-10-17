import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { Window as HappyDOMWindow } from "happy-dom";
import { expect, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import type { WorkspaceState } from "@/lib/state/store";
import type { WorkspacePayload } from "@/lib/types/backend";
import { registerWorkspacesHandlers } from "@/lib/ws/handlers/workspaces";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import WorkspacesPage from "./page";

export type Item = WorkspaceState["items"][number];
export type Store = ReturnType<typeof useAppStoreApi>;
export const BASE = "base-workspace";
export const OTHER = "other-workspace";
export const ACCEPTED = "accepted-workspace";
export const INDEPENDENT = "independent-workspace";
// i18n-exempt: Synthetic HTTP error body for the rejected-creation integration control.
export const REJECTION = "Creation rejected by server";

export function descriptor(id: string, values: Partial<Item> = {}): Item {
  return {
    id,
    name: id,
    description: id,
    owner_id: "owner-id",
    unit_id: "unit-id",
    viewer_role: "owner",
    scopes: ["workspace.manage"],
    member_count: 3,
    default_executor_id: "executor-id",
    default_environment_id: "environment-id",
    default_agent_profile_id: "agent-profile-id",
    default_config_agent_profile_id: "config-profile-id",
    acp_idle_suspension_enabled: true,
    acp_idle_timeout_minutes: 45,
    office_workflow_id: null,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-02T00:00:00Z",
    ...values,
  };
}

type Ticket = {
  payload: unknown;
  settled: boolean;
  finish: (value: Item, status?: number) => void;
};
export type Fixture = {
  store: Store;
  tickets: Ticket[];
  unexpected: string[];
  reads: Promise<Response>[];
  countRequests: number;
  view: ReturnType<typeof render>;
};
const fixtures: Fixture[] = [];
let client: WebSocketClient | undefined;
let previousClient: ReturnType<typeof getWebSocketClient>;
let originalPath: string;
let originalCookie: string;

class CountSocket {
  static current: CountSocket;
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  constructor() {
    CountSocket.current = this;
  }
  send(data: string) {
    const message = JSON.parse(data) as { id: string; action: string };
    if (message.action !== "automation.list") {
      fixtures[0].unexpected.push(message.action);
    }
    queueMicrotask(() => {
      this.onmessage?.({
        data: JSON.stringify({ id: message.id, type: "response", payload: [] }),
      } as MessageEvent);
    });
  }
  close() {
    this.onclose?.({ code: 1000 } as CloseEvent);
  }
}

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function createTicket(current: Fixture, body: BodyInit | null | undefined) {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((done) => {
    resolve = done;
  });
  const ticket: Ticket = {
    payload: JSON.parse(String(body)),
    settled: false,
    finish(value, status = 200) {
      if (ticket.settled) return;
      ticket.settled = true;
      resolve(json(status === 200 ? value : { error: REJECTION }, status));
    },
  };
  current.tickets.push(ticket);
  return promise;
}

function countResponse(path: string): unknown {
  if (/^\/api\/v1\/workspaces\/[^/]+\/repositories$/.test(path)) return { repositories: [] };
  if (path === "/api/v1/workflows") return { workflows: [] };
  if (path === "/api/v1/secrets") return [];
  if (path === "/api/v1/github/status") return { authenticated: false };
  if (path === "/api/v1/sentry/instances") return { instances: [] };
  if (/^\/api\/v1\/(azure-devops|gitlab|jira|linear)\/config$/.test(path)) return null;
  return undefined;
}

function installTransport() {
  originalPath = window.location.pathname + window.location.search;
  originalCookie = document.cookie;
  previousClient = getWebSocketClient();
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", "/settings/workspaces");
  vi.stubGlobal("WebSocket", CountSocket);
  client = new WebSocketClient("ws://localhost/counts", undefined, { enabled: false });
  client.connect();
  CountSocket.current.onopen?.();
  setWebSocketClient(client);
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), window.location.origin);
    const method = init?.method ?? "GET";
    const current = fixtures[0];
    if (url.pathname === "/api/v1/workspaces" && method === "POST")
      return createTicket(current, init?.body);
    if (method === "POST" && url.pathname === "/api/v1/system/logs/frontend-errors")
      return Promise.resolve(json({}));
    const value = countResponse(url.pathname);
    if (method === "GET" && value !== undefined) {
      current.countRequests += 1;
      const promise = Promise.resolve(json(value));
      current.reads.push(promise);
      return promise;
    }
    current.unexpected.push(`${method} ${url.pathname}`);
    // i18n-exempt: Diagnostic for an unexpected external route in this integration fixture.
    throw new Error(`Unexpected transport ${method} ${url.pathname}`);
  });
}

function Observer({ fixture }: { fixture: Fixture }) {
  fixture.store = useAppStoreApi();
  return null;
}

export async function mountCatalogue(
  items = [descriptor(BASE), descriptor(OTHER)],
  activeId: string | null = BASE,
  activeIdRevision = 7,
  page = true,
): Promise<Fixture> {
  const current = {
    tickets: [],
    unexpected: [],
    reads: [],
    countRequests: 0,
  } as unknown as Fixture;
  fixtures.push(current);
  if (fixtures.length === 1) installTransport();
  current.view = render(
    <StateProvider initialState={{ workspaces: { items, activeId, activeIdRevision } }}>
      <ToastProvider>
        <TooltipProvider>
          <Observer fixture={current} />
          {page && <WorkspacesPage />}
          {page && <AppSidebarWorkspacePicker modal={false} />}
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  await act(async () => {
    await Promise.all(fixtures[0].reads);
  });
  expect(current.unexpected).toEqual([]);
  return current;
}

export function openForm() {
  fireEvent.click(screen.getByRole("button", { name: "Add Workspace" }));
}

// i18n-exempt: Synthetic form input exercises name trimming, not product copy.
export async function beginCreation(current: Fixture, name = "  Accepted workspace  ") {
  openForm();
  fireEvent.change(screen.getByLabelText("Workspace Name"), { target: { value: name } });
  fireEvent.submit(screen.getByLabelText("Workspace Name").closest("form")!);
  await waitFor(() => expect(current.tickets).toHaveLength(1));
  return current.tickets[0];
}

export async function finishCreation(current: Fixture, value = descriptor(ACCEPTED), status = 200) {
  await act(async () => {
    current.tickets[0].finish(value, status);
    await Promise.all(current.reads);
  });
  if (status === 200)
    await waitFor(() => expect(screen.queryByLabelText("Workspace Name")).toBeNull());
  else await screen.findByText(REJECTION);
  expect(current.unexpected).toEqual([]);
}

export function notify(
  current: Fixture,
  action: "workspace.created" | "workspace.updated" | "workspace.deleted",
  payload: WorkspacePayload,
) {
  act(() => {
    const handlers = registerWorkspacesHandlers(current.store);
    if (action === "workspace.created")
      handlers[action]!({ type: "notification", action, payload });
    if (action === "workspace.updated")
      handlers[action]!({ type: "notification", action, payload });
    if (action === "workspace.deleted")
      handlers[action]!({ type: "notification", action, payload });
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
export function managementChoice(id: string) {
  return (
    screen
      .queryAllByTestId("workspace-overview-link")
      .find((link) => link.getAttribute("href")?.endsWith(`/${id}`)) ?? null
  );
}
export function catalogue(current: Fixture) {
  return current.store.getState().workspaces;
}
export function displayedNames() {
  return screen
    .getAllByTestId("workspace-list-item")
    .map((card) => within(card).getByRole("heading").textContent);
}

export async function cleanupCatalogue() {
  await act(async () => {
    for (const current of fixtures) {
      for (const ticket of current.tickets) ticket.finish(descriptor(ACCEPTED));
      await Promise.all(current.reads);
    }
  });
  cleanup();
  client?.disconnect();
  setWebSocketClient(previousClient);
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", originalPath);
  for (const entry of document.cookie.split(";"))
    document.cookie = `${entry.split("=")[0].trim()}=; path=/; max-age=0`;
  for (const entry of originalCookie.split(";"))
    if (entry.trim()) document.cookie = `${entry.trim()}; path=/`;
  await (window as unknown as HappyDOMWindow).happyDOM.abort();
  vi.unstubAllGlobals();
  const unexpected = fixtures.flatMap((current) => current.unexpected);
  fixtures.length = 0;
  client = undefined;
  expect(unexpected).toEqual([]);
}
