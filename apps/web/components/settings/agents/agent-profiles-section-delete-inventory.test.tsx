import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { CreateEditSelectors } from "@/components/task-create-dialog-form-body";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import {
  computeAgentCompatState,
  filterCompatibleAgentProfiles,
} from "@/components/task-create-dialog-computed";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { SelectorsRow } from "@/components/task/new-subtask-form-parts";
import { ApiError, fetchJson } from "@/lib/api/client";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import type { AppState } from "@/lib/state/store";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AgentProfilePayload } from "@/lib/types/backend";
import type { Agent, AvailableAgent } from "@/lib/types/http";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { ProfileRow } from "./agent-profiles-section";

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

const TARGET = "delete-inventory-target";
const SIBLING = "delete-inventory-sibling";
const OWNER = "delete-inventory-owner";
const MODEL = "inventory-model";
const RECEIVED_NAME = "Received reviewer";
const VERSION = 41;
const TARGET_NAME = "Delete target";
const SIBLING_NAME = "Retained sibling";
let sequence = 0;
let timestamp = "";
type WireProfile = AgentProfilePayload & {
  kind: "concrete";
  enabled: boolean;
  cli_flags: [];
  workspace_id?: string;
};

const pendingDeletes = new Map<string, ReturnType<typeof deferredDelete>>();

function deferredDelete() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function wireProfile(id: string, name: string, agentId = OWNER): WireProfile {
  return {
    id,
    agent_id: agentId,
    agent_display_name: "Codex",
    name,
    kind: "concrete",
    model: MODEL,
    enabled: true,
    allow_indexing: false,
    auto_approve: false,
    dangerously_skip_permissions: false,
    plan: "",
    cli_flags: [],
    cli_passthrough: false,
    created_at: timestamp,
    updated_at: timestamp,
  };
}

function settingsAgent(): Agent {
  return {
    id: OWNER,
    name: "codex",
    supports_mcp: true,
    mcp_config_path: "",
    inference_capable: true,
    capability_status: "ok",
    profiles: [wireProfile(TARGET, TARGET_NAME), wireProfile(SIBLING, SIBLING_NAME)].map(
      normalizeAgentProfile,
    ),
    created_at: timestamp,
    updated_at: timestamp,
  };
}

function availableAgent(): AvailableAgent {
  return {
    name: "codex",
    display_name: "Codex",
    supports_mcp: true,
    installation_paths: [],
    available: true,
    capabilities: {
      supports_session_resume: false,
      supports_shell: false,
      supports_workspace_only: false,
    },
    model_config: {
      status: "ok",
      supports_dynamic_models: false,
      default_model: MODEL,
      available_models: [{ id: MODEL, name: "Inventory model" }],
    },
    permission_settings: {},
    updated_at: timestamp,
  };
}

function TaskChoices() {
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const [selected, setSelected] = useState("");
  const compatible = filterCompatibleAgentProfiles(profiles, null, true, []);
  const options = useAgentProfileOptions(compatible, "task_create");
  const selectedProfile = profiles.find((profile) => profile.id === selected) ?? null;
  const compat = computeAgentCompatState({
    selectedExecutorProfile: null,
    compatibleAgentProfiles: compatible,
    selectedAgentProfileId: selected,
    selectedAgentProfile: selectedProfile,
    workflowAgentLocked: false,
    dynamicRoutingEnabled: true,
  });
  return (
    <section aria-label="Task choices">
      <CreateEditSelectors
        isTaskStarted={false}
        agentProfiles={profiles}
        agentProfilesLoading={false}
        agentProfileOptions={options}
        agentProfileId={selected}
        onAgentProfileChange={setSelected}
        isCreatingSession={false}
        executorProfileOptions={[]}
        executorProfileId=""
        onExecutorProfileChange={() => undefined}
        executorsLoading={false}
        AgentSelectorComponent={AgentSelector}
        ExecutorProfileSelectorComponent={ExecutorProfileSelector}
        workflowAgentLocked={false}
        agentCompatState={compat}
        selectedAgentProfileName={selectedProfile?.label ?? null}
        effectiveWorkflowName={null}
        executorProfileName={null}
        runnerEditable
        runnerIneligibleReason=""
      />
    </section>
  );
}

function SubtaskChoices() {
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const options = useAgentProfileOptions(profiles, "task_create");
  const [selected, setSelected] = useState("");
  return (
    <section aria-label="Subtask choices">
      <SelectorsRow
        profileOptions={options}
        executorProfileOptions={[]}
        agentProfileId={selected}
        executorProfileId=""
        onAgentProfileChange={setSelected}
        onExecutorProfileChange={() => undefined}
        disabled={false}
        hideExecutor
      />
    </section>
  );
}

function mountInventory() {
  const agent = settingsAgent();
  let store!: StoreApi<AppState>;
  function Inventory() {
    store = useAppStoreApi();
    const agents = useAppStore((state) => state.settingsAgents.items);
    return (
      <>
        {agents.flatMap((owner) =>
          owner.profiles.map((profile) => (
            <ProfileRow key={profile.id} agent={owner} profile={profile} />
          )),
        )}
        <TaskChoices />
        <SubtaskChoices />
      </>
    );
  }
  render(
    <StateProvider
      initialState={{
        auth: {
          mode: "enabled",
          authenticated: true,
          user: {
            id: "inventory-admin",
            email: "admin@example.test",
            display_name: "Admin",
            role: "admin",
            status: "active",
          },
        },
        settingsAgents: { items: [agent] },
        agentProfiles: {
          orderByAgent: {},
          items: agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
          version: VERSION,
        },
        availableAgents: { items: [availableAgent()], tools: [], loaded: true, loading: false },
        agentProfileRecentUse: { records: {}, loaded: true },
      }}
    >
      <TooltipProvider>
        <ToastProvider>
          <Inventory />
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
  return { store, handlers: registerAgentsHandlers(store) };
}

type Inventory = ReturnType<typeof mountInventory>;

async function confirmDelete(id = TARGET) {
  fireEvent.click(screen.getByTestId(`delete-profile-inline-${id}`));
  fireEvent.click(await screen.findByTestId("agent-profile-delete-confirm"));
  await waitFor(() => expect(pendingDeletes.has(id)).toBe(true));
  return pendingDeletes.get(id)!;
}

function receiveGlobal(
  fixture: Inventory,
  received = wireProfile(`received-${sequence}`, RECEIVED_NAME, `absent-owner-${sequence}`),
) {
  act(() =>
    fixture.handlers["agent.profile.created"]?.({
      type: "notification",
      action: "agent.profile.created",
      timestamp,
      payload: { profile: received, inference_capable: true },
    }),
  );
  expect(fixture.store.getState().settingsAgents.items.map((agent) => agent.id)).toEqual([OWNER]);
  return received;
}

async function selectChoice(surface: "Task choices" | "Subtask choices", name: string) {
  const trigger = within(screen.getByRole("region", { name: surface })).getByTestId(
    "agent-profile-selector",
  );
  fireEvent.click(trigger);
  const option = await screen.findByRole("option", { name: new RegExp(name) });
  expect(option.getAttribute("aria-disabled")).not.toBe("true");
  fireEvent.click(option);
  await waitFor(() => expect(trigger.textContent).toContain(name));
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  return trigger;
}

async function selectBoth(name: string) {
  return [await selectChoice("Task choices", name), await selectChoice("Subtask choices", name)];
}

async function assertChoices(names: string[], excluded: string[] = []) {
  for (const surface of ["Task choices", "Subtask choices"]) {
    const trigger = within(screen.getByRole("region", { name: surface })).getByTestId(
      "agent-profile-selector",
    );
    fireEvent.click(trigger);
    for (const name of names) {
      expect(await screen.findByRole("option", { name: new RegExp(name) })).toBeTruthy();
    }
    for (const name of excluded)
      expect(screen.queryByRole("option", { name: new RegExp(name) })).toBeNull();
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  }
}

beforeEach(() => {
  sequence += 1;
  timestamp = new Date(Date.UTC(2026, 9, 10, 0, sequence)).toISOString();
  window.history.replaceState({}, "", "/settings/agents");
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1280 });
  vi.mocked(fetchJson).mockReset();
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(url: string, options?: Parameters<typeof fetchJson>[1]) => {
      const id = new URL(url).pathname.split("/").at(-1)!;
      if (options?.init?.method !== "DELETE" || ![TARGET, SIBLING].includes(id)) {
        throw new Error(`Unexpected transport request: ${url}`);
      }
      const pending = deferredDelete();
      pendingDeletes.set(id, pending);
      return pending.promise as Promise<T>;
    },
  );
});

afterEach(async () => {
  await act(async () => {
    for (const pending of pendingDeletes.values()) pending.resolve();
    await Promise.all(
      [...pendingDeletes.values()].map((pending) => pending.promise.catch(() => undefined)),
    );
  });
  pendingDeletes.clear();
  cleanup();
  window.history.replaceState({}, "", "/");
});

describe("ProfileRow deletion inventory", () => {
  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.1, .5
  it("retains an absent-owner global choice after delete ACK", async () => {
    const fixture = mountInventory();
    const pending = await confirmDelete();
    const received = receiveGlobal(fixture);
    const triggers = await selectBoth(RECEIVED_NAME);
    const before = fixture.store
      .getState()
      .agentProfiles.items.find((option) => option.id === received.id);
    const version = fixture.store.getState().agentProfiles.version;
    await act(async () => pending.resolve());
    await waitFor(() =>
      expect(
        fixture.store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id),
      ).toEqual([SIBLING]),
    );
    for (const trigger of triggers) expect(trigger.textContent).toContain(RECEIVED_NAME);
    expect(
      fixture.store.getState().agentProfiles.items.find((option) => option.id === received.id),
    ).toEqual(before);
    expect(fixture.store.getState().agentProfiles.items.map((option) => option.id)).toEqual([
      SIBLING,
      received.id,
    ]);
    expect(fixture.store.getState().agentProfiles.version).toBe(version + 1);
    await assertChoices([RECEIVED_NAME, SIBLING_NAME], [TARGET_NAME]);
  });

  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.1
  it("removes only the accepted target without an intervening event", async () => {
    const fixture = mountInventory();
    const triggers = await selectBoth(SIBLING_NAME);
    const pending = await confirmDelete();
    await act(async () => pending.resolve());
    await waitFor(() =>
      expect(fixture.store.getState().agentProfiles.items.map((option) => option.id)).toEqual([
        SIBLING,
      ]),
    );
    expect(
      fixture.store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id),
    ).toEqual([SIBLING]);
    for (const trigger of triggers) expect(trigger.textContent).toContain(SIBLING_NAME);
    expect(fixture.store.getState().agentProfiles.version).toBe(VERSION + 1);
  });

  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.4, .5
  it("preserves received choices after rejected deletion", async () => {
    const fixture = mountInventory();
    const pending = await confirmDelete();
    receiveGlobal(fixture);
    const triggers = await selectBoth(RECEIVED_NAME);
    const before = fixture.store.getState();
    await act(async () => pending.reject(new ApiError("Delete rejected", 500, {})));
    await screen.findByText("Delete rejected");
    expect(fixture.store.getState().settingsAgents).toBe(before.settingsAgents);
    expect(fixture.store.getState().agentProfiles).toBe(before.agentProfiles);
    for (const trigger of triggers) expect(trigger.textContent).toContain(RECEIVED_NAME);
    expect(window.location.pathname).toBe("/settings/agents");
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId(`delete-profile-inline-${TARGET}`)),
    );
  });
});

describe("ProfileRow deletion metadata and completion order", () => {
  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.2, .5
  it("preserves newer option metadata and mixed eligibility", async () => {
    const fixture = mountInventory();
    const pending = await confirmDelete();
    receiveGlobal(fixture);
    const future = new Date(Date.parse(timestamp) + 1000).toISOString();
    act(() =>
      fixture.store.getState().setAgentProfiles(
        fixture.store.getState().agentProfiles.items.map((option) =>
          option.id === SIBLING
            ? {
                ...option,
                label: "Codex • Updated disabled sibling",
                model: "newer-model",
                enabled: false,
                updatedAt: future,
                fallback_model: "newer-fallback",
                auto_fallback: true,
                require_exact_model: true,
                capability_error: "Current capability detail",
              }
            : option,
        ),
      ),
    );
    receiveGlobal(fixture, {
      ...wireProfile(`office-${sequence}`, "Office-only profile"),
      workspace_id: "office-workspace",
    });
    const before = fixture.store.getState();
    const options = before.agentProfiles.items.filter((option) => option.id !== TARGET);
    expect(
      before.settingsAgents.items[0].profiles.find((profile) => profile.id === SIBLING)?.enabled,
    ).toBe(true);
    await assertChoices([RECEIVED_NAME], ["Updated disabled sibling", "Office-only profile"]);
    await act(async () => pending.resolve());
    expect(fixture.store.getState().agentProfiles.items).toEqual(options);
    expect(fixture.store.getState().agentProfiles.version).toBe(before.agentProfiles.version + 1);
    expect(fixture.store.getState().settingsAgents.items[0]).toEqual({
      ...before.settingsAgents.items[0],
      profiles: before.settingsAgents.items[0].profiles.filter((profile) => profile.id !== TARGET),
    });
    await assertChoices(
      [RECEIVED_NAME],
      ["Updated disabled sibling", "Office-only profile", TARGET_NAME],
    );
  });

  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.3
  it.each([
    [TARGET, SIBLING],
    [SIBLING, TARGET],
  ])("does not resurrect targets when deletions complete %s then %s", async (first, second) => {
    const fixture = mountInventory();
    const targetDelete = await confirmDelete(TARGET);
    const siblingDelete = await confirmDelete(SIBLING);
    const received = receiveGlobal(fixture);
    const triggers = await selectBoth(RECEIVED_NAME);
    const options = fixture.store.getState().agentProfiles.items;
    const deletes = new Map([
      [TARGET, targetDelete],
      [SIBLING, siblingDelete],
    ]);
    await act(async () => deletes.get(first)!.resolve());
    expect(fixture.store.getState().agentProfiles.items).toEqual(
      options.filter((option) => option.id !== first),
    );
    await act(async () => deletes.get(second)!.resolve());
    expect(fixture.store.getState().settingsAgents.items[0].profiles).toEqual([]);
    expect(fixture.store.getState().agentProfiles.items).toEqual(
      options.filter((option) => option.id === received.id),
    );
    for (const trigger of triggers) expect(trigger.textContent).toContain(RECEIVED_NAME);
  });
});

describe("ProfileRow deletion failure and management controls", () => {
  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.4
  it("preserves choices and navigates to guided resolution on conflict", async () => {
    const fixture = mountInventory();
    const pending = await confirmDelete();
    receiveGlobal(fixture);
    const triggers = await selectBoth(RECEIVED_NAME);
    const before = fixture.store.getState();
    await act(async () =>
      pending.reject(new ApiError("In use", 409, { watchers: [{ id: "watcher-reference" }] })),
    );
    await waitFor(() =>
      expect(window.location.pathname).toBe(`/settings/agents/codex/profiles/${TARGET}`),
    );
    expect(fixture.store.getState().settingsAgents).toBe(before.settingsAgents);
    expect(fixture.store.getState().agentProfiles).toBe(before.agentProfiles);
    for (const trigger of triggers) expect(trigger.textContent).toContain(RECEIVED_NAME);
  });

  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.4
  it("preserves received choices after handled deletion without another error toast", async () => {
    const fixture = mountInventory();
    const pending = await confirmDelete();
    receiveGlobal(fixture);
    const before = fixture.store.getState();
    const handled = new ApiError("Already handled", 403, {});
    handled.handled = true;
    await act(async () => pending.reject(handled));
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId(`delete-profile-inline-${TARGET}`)),
    );
    expect(screen.queryByText("Already handled")).toBeNull();
    expect(fixture.store.getState().settingsAgents).toBe(before.settingsAgents);
    expect(fixture.store.getState().agentProfiles).toBe(before.agentProfiles);
    await selectBoth(RECEIVED_NAME);
  });

  // @covers AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.6
  it("cancels confirmation without deleting and withholds management controls from a member", async () => {
    const fixture = mountInventory();
    const trigger = screen.getByTestId(`delete-profile-inline-${TARGET}`);
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(fetchJson).not.toHaveBeenCalled();
    act(() =>
      fixture.store.setState((state) => ({
        auth: {
          ...state.auth,
          user: { ...state.auth.user!, role: "member" },
        },
      })),
    );
    expect(screen.queryByTestId(`delete-profile-inline-${TARGET}`)).toBeNull();
    expect(screen.queryByTestId(`duplicate-profile-inline-${TARGET}`)).toBeNull();
    expect(screen.getByRole("link", { name: TARGET_NAME })).toBeTruthy();
  });
});

it("rejects a list snapshot started before an accepted deletion without a WS event", async () => {
  const fixture = mountInventory();
  const before = fixture.store.getState();
  const pending = await confirmDelete();
  await act(async () => pending.resolve());
  const deleted = fixture.store.getState();
  expect(deleted.agentProfiles.version).toBe(before.agentProfiles.version + 1);
  act(() => {
    expect(
      fixture.store
        .getState()
        .applyAgentListSnapshot(before.settingsAgents.items, before.agentProfiles.version),
    ).toBe(false);
  });
  expect(fixture.store.getState().agentProfiles.items.map((profile) => profile.id)).toEqual([
    SIBLING,
  ]);
  expect(
    fixture.store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id),
  ).toEqual([SIBLING]);
});
