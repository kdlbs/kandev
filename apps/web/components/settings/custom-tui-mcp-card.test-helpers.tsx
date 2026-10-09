import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { StoreApi } from "zustand";
import { expect, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { AgentProfilesSubList } from "@/components/settings/agents/agent-profiles-section";
import { CustomTUIMcpCard } from "@/components/settings/custom-tui-mcp-card";
import { fetchJson } from "@/lib/api/client";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AppState } from "@/lib/state/store";
import type { Agent, AgentProfilePayload } from "@/lib/types/http-agents";
import type { AgentProfilePayload as WsAgentProfilePayload } from "@/lib/types/backend";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";

export const A = "terminal-a";
export const B = "terminal-b";
const STAMP = "2026-10-09T00:00:00Z";
const LATER = "2026-10-09T00:01:00Z";
let fixtureNumber = 0;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

export function profileWire(
  id: string,
  agentId: string,
  name: string,
): AgentProfilePayload & WsAgentProfilePayload {
  return {
    id,
    agent_id: agentId,
    agent_display_name: agentId,
    name,
    model: "initial-model",
    enabled: true,
    cli_passthrough: true,
    allow_indexing: false,
    auto_approve: false,
    dangerously_skip_permissions: false,
    plan: "",
    cli_flags: [],
    created_at: STAMP,
    updated_at: LATER,
  };
}

export function owner(id: string, profileId: string): Agent {
  return {
    id,
    name: id,
    supports_mcp: false,
    tui_config: { command: id, display_name: id, wait_for_terminal: true },
    profiles: [normalizeAgentProfile(profileWire(profileId, id, `${id} profile`))],
    created_at: STAMP,
    updated_at: STAMP,
  };
}

type Pending = {
  response: ReturnType<typeof deferred<unknown>>;
  admitted: ReturnType<typeof deferred<void>>;
};
export type Fixture = {
  store: StoreApi<AppState>;
  initial: Agent[];
  profileIds: { a: string; b: string; received: string };
  pending: Map<string, Pending>;
};
const fixtures: Fixture[] = [];

export async function mountCards(): Promise<Fixture> {
  const prefix = `mcp-ack-${++fixtureNumber}`;
  const profileIds = { a: `${prefix}-a`, b: `${prefix}-b`, received: `${prefix}-received` };
  const initial = [owner(A, profileIds.a), owner(B, profileIds.b)];
  const pending = new Map(
    [A, B].map((id) => [
      id,
      {
        response: deferred<unknown>(),
        admitted: deferred<void>(),
      },
    ]),
  );
  const fixture = { initial, profileIds, pending } as Fixture;
  fixtures.push(fixture);
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(url: string, options?: Parameters<typeof fetchJson>[1]) => {
      if (url === "/api/v1/agents/tui/mcp-strategies") {
        return {
          strategies: [
            // i18n-exempt: Backend-provided description in a transport-only test fixture.
            { key: "claude", description: "Claude configuration" },
            // i18n-exempt: Backend-provided description in a transport-only test fixture.
            { key: "codex", description: "Codex arguments" },
          ],
        } as T;
      }
      for (const [id, request] of pending) {
        if (url === `/api/v1/agents/tui/${id}/mcp` && options?.init?.method === "PATCH") {
          request.admitted.resolve();
          return request.response.promise as Promise<T>;
        }
      }
      if (url === "/api/v1/system/logs/frontend-errors") return undefined as T;
      throw new Error(`Unexpected transport: ${url}`);
    },
  );
  function CurrentCards() {
    fixture.store = useAppStoreApi();
    const agents = useAppStore((state) => state.settingsAgents.items);
    return agents.map((agent) => (
      <section key={agent.id} data-testid={`owner-${agent.id}`}>
        <AgentProfilesSubList
          savedAgent={agent}
          agentName={agent.name}
          canManage
          onReorder={vi.fn()}
        />
        <CustomTUIMcpCard agent={agent} />
      </section>
    ));
  }
  render(
    <StateProvider
      initialState={{
        settingsAgents: { items: initial },
        agentProfiles: {
          orderByAgent: {},
          items: initial.flatMap((agent) =>
            agent.profiles.map((p) => toAgentProfileOption(agent, p)),
          ),
          version: 0,
        },
      }}
    >
      <TooltipProvider>
        <ToastProvider>
          <CurrentCards />
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  return fixture;
}

export function trigger(id = A) {
  return within(screen.getByTestId(`owner-${id}`)).getByTestId("mcp-strategy-select");
}

export async function startSave(fixture: Fixture, id = A, key = "claude") {
  fireEvent.click(trigger(id));
  fireEvent.click(screen.getByRole("option", { name: key || /^Off:/ }));
  await act(async () => {
    await fixture.pending.get(id)!.admitted.promise;
  });
  expect(trigger(id).getAttribute("disabled")).not.toBeNull();
  const call = vi
    .mocked(fetchJson)
    .mock.calls.find(([url]) => url === `/api/v1/agents/tui/${id}/mcp`);
  expect(JSON.parse(call![1]!.init!.body as string)).toEqual({ mcp_strategy: key });
}

export function accepted(fixture: Fixture, id = A, key: string | undefined = "claude") {
  const original = fixture.initial.find((agent) => agent.id === id)!;
  return {
    ...original,
    supports_mcp: Boolean(key),
    tui_config: { ...original.tui_config!, mcp_strategy: key },
    profiles: original.profiles.map((profile) => profileWire(profile.id, id, profile.name)),
  };
}

export async function accept(fixture: Fixture, response: unknown = accepted(fixture), id = A) {
  await act(async () => {
    fixture.pending.get(id)!.response.resolve(response);
  });
}

export function publish(fixture: Fixture, agents: Agent[]) {
  act(() => fixture.store.getState().setSettingsAgents(agents));
  expect(fixture.store.getState().settingsAgents.items).toEqual(agents);
}

export function profileEvent(
  fixture: Fixture,
  action: "created" | "updated" | "deleted",
  profile: AgentProfilePayload,
) {
  const handlers = registerAgentsHandlers(fixture.store);
  const key = `agent.profile.${action}` as const;
  act(() =>
    handlers[key]?.({
      id: `${profile.id}-${action}`,
      type: "notification",
      action: key,
      timestamp: LATER,
      payload: { profile },
    } as never),
  );
}

export async function cleanupCards() {
  await act(async () => {
    for (const fixture of fixtures.splice(0)) {
      for (const [id, request] of fixture.pending) request.response.resolve(accepted(fixture, id));
    }
  });
  cleanup();
  await act(async () => {
    await vi.runOnlyPendingTimersAsync();
  });
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.mocked(fetchJson).mockReset();
}
