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
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { ApiError, fetchJson } from "@/lib/api/client";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type { AppState } from "@/lib/state/store";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AgentProfilePayload } from "@/lib/types/backend";
import type { Agent, AvailableAgent } from "@/lib/types/http";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { AgentProfilePage } from "./agent-profile-page";
import { AgentProfilePicker } from "./agent-profile-picker";

// Only external HTTP is deferred. The page, save coordinator, PATCH adapter,
// response acceptance, normalizer, live handler, store and picker stay real.
vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

const PROFILE_CREATED = "agent.profile.created";
const PROFILE_UPDATED = "agent.profile.updated";
const PROFILE_DELETED = "agent.profile.deleted";
const PROFILE_NAME_INPUT = "profile-name-input";
const INDEPENDENT_NAME = "Latest independent";

const MODEL = "catalogue-model";
const PICKER = "save-catalogue-picker";
const SAVED_NAME = "Saved editor";
let sequence = 0;
const pendingRequests: Array<ReturnType<typeof deferred>> = [];
type WireProfile = AgentProfilePayload & {
  kind: "concrete";
  enabled: boolean;
  cli_flags: [];
  workspace_id?: string;
};

function deferred() {
  let resolve!: (value: unknown) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<unknown>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.mocked(fetchJson).mockReset();
});
afterEach(async () => {
  await act(async () => {
    for (const request of pendingRequests.splice(0)) request.resolve({});
  });
  cleanup();
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", "/");
});

function wireProfile(id: string, name: string, ownerId: string, revision: string): WireProfile {
  return {
    id,
    agent_id: ownerId,
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
    created_at: revision,
    updated_at: revision,
  };
}

function availableAgent(revision: string): AvailableAgent {
  return {
    name: "codex",
    display_name: "Codex",
    supports_mcp: false,
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
      available_models: [{ id: MODEL, name: "Catalogue model" }],
    },
    permission_settings: {},
    updated_at: revision,
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

function editorFixture() {
  sequence += 1;
  const ownerId = `save-owner-${sequence}`;
  const targetId = `save-target-${sequence}`;
  const siblingId = `save-sibling-${sequence}`;
  const revision = new Date(Date.UTC(2026, 9, 10, 3, sequence, 0)).toISOString();
  const ackRevision = new Date(Date.UTC(2026, 9, 10, 3, sequence, 1)).toISOString();
  const liveRevision = new Date(Date.UTC(2026, 9, 10, 3, sequence, 2)).toISOString();
  const target = wireProfile(targetId, "Initial editor", ownerId, revision);
  const sibling = wireProfile(siblingId, "Deleted sibling", ownerId, revision);
  const owner: Agent = {
    id: ownerId,
    name: "codex",
    supports_mcp: false,
    mcp_config_path: "",
    inference_capable: true,
    capability_status: "ok",
    profiles: [target, sibling].map(normalizeAgentProfile),
    created_at: revision,
    updated_at: revision,
  };
  const pending = deferred();
  return {
    ownerId,
    targetId,
    siblingId,
    revision,
    ackRevision,
    liveRevision,
    target,
    sibling,
    owner,
    pending,
  };
}

function renderEditor(fixture: ReturnType<typeof editorFixture>) {
  const { owner, targetId, siblingId, revision } = fixture;
  let store!: StoreApi<AppState>;
  let coordinator!: SettingsSaveCoordinator;
  function Consumer() {
    store = useAppStoreApi();
    coordinator = useSettingsSaveCoordinator();
    const options = useAppStore((state) => state.agentProfiles.items);
    return (
      <>
        <AgentProfilePage />
        <AgentProfilePicker
          profiles={options}
          value={siblingId}
          onValueChange={() => undefined}
          testId={PICKER}
        />
        <TaskChoices />
        <SubtaskChoices />
      </>
    );
  }
  window.history.replaceState({}, "", `/settings/agents/codex/profiles/${targetId}`);
  render(
    <StateProvider
      initialState={{
        auth: {
          mode: "enabled",
          authenticated: true,
          user: {
            id: "save-admin",
            email: "save@example.test",
            display_name: "Admin",
            role: "admin",
            status: "active",
          },
        },
        settingsAgents: { items: [owner] },
        agentProfiles: {
          orderByAgent: {},
          items: owner.profiles.map((profile) => toAgentProfileOption(owner, profile)),
          version: 13,
        },
        availableAgents: {
          items: [availableAgent(revision)],
          tools: [],
          loaded: true,
          loading: false,
        },
        secrets: { items: [], loaded: true, loading: false },
        agentProfileRecentUse: { records: {}, loaded: true },
      }}
    >
      <TooltipProvider>
        <ToastProvider>
          <SettingsSaveProvider>
            <Consumer />
          </SettingsSaveProvider>
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
  return {
    get store() {
      return store;
    },
    get coordinator() {
      return coordinator;
    },
  };
}

function mountEditor() {
  const fixture = editorFixture();
  const {
    ownerId,
    targetId,
    siblingId,
    ackRevision,
    liveRevision,
    target,
    sibling,
    owner,
    pending,
  } = fixture;
  pendingRequests.push(pending);
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(url: string, options?: Parameters<typeof fetchJson>[1]) => {
      if (url.endsWith(`/agent-profiles/${targetId}`) && options?.init?.method === "PATCH") {
        return pending.promise as Promise<T>;
      }
      if (url.includes("command-preview")) return { command_string: "codex", command: [] } as T;
      if (url.endsWith("/agents"))
        return { agents: view.store.getState().settingsAgents.items } as T;
      throw new Error(`Unexpected external request: ${url}`);
    },
  );
  const view = renderEditor(fixture);
  const handlers = registerAgentsHandlers(view.store);
  function receive(
    action: typeof PROFILE_CREATED | typeof PROFILE_UPDATED | typeof PROFILE_DELETED,
    profile: WireProfile,
  ) {
    const message = {
      type: "notification" as const,
      timestamp: profile.updated_at,
      payload: { profile, inference_capable: true },
    };
    act(() => {
      if (action === PROFILE_CREATED) handlers[action]?.({ ...message, action });
      else if (action === PROFILE_UPDATED) handlers[action]?.({ ...message, action });
      else handlers[action]?.({ ...message, action });
    });
  }
  function removeSibling() {
    act(() =>
      handlers[PROFILE_DELETED]?.({
        type: "notification",
        action: PROFILE_DELETED,
        timestamp: liveRevision,
        payload: { profile: { ...sibling, updated_at: liveRevision } },
      }),
    );
  }
  async function beginSave() {
    fireEvent.change(screen.getByTestId(PROFILE_NAME_INPUT), { target: { value: SAVED_NAME } });
    await waitFor(() => expect(view.coordinator.hasDirty).toBe(true));
    let saving!: ReturnType<SettingsSaveCoordinator["saveAll"]>;
    act(() => {
      saving = view.coordinator.saveAll();
    });
    await waitFor(() =>
      expect(
        vi.mocked(fetchJson).mock.calls.some(([, options]) => options?.init?.method === "PATCH"),
      ).toBe(true),
    );
    return { saving };
  }
  async function finishSave(saving: ReturnType<SettingsSaveCoordinator["saveAll"]>) {
    let result!: Awaited<typeof saving>;
    await act(async () => {
      pending.resolve({ ...target, name: SAVED_NAME, updated_at: ackRevision });
      result = await saving;
    });
    return result;
  }
  return {
    get store() {
      return view.store;
    },
    get coordinator() {
      return view.coordinator;
    },
    targetId,
    siblingId,
    ownerId,
    ackRevision,
    liveRevision,
    target,
    sibling,
    owner,
    pending,
    receive,
    removeSibling,
    beginSave,
    finishSave,
  };
}

function nestedProfile(h: ReturnType<typeof mountEditor>, id: string) {
  return h.store
    .getState()
    .settingsAgents.items.flatMap((owner) => owner.profiles)
    .find((profile) => profile.id === id);
}

async function assertUnavailable() {
  expect(screen.getByTestId(PICKER).textContent).toContain("Unavailable");
  fireEvent.click(screen.getByTestId(PICKER));
  const unavailable = await screen.findByRole("option", { name: /Unavailable/ });
  expect(unavailable.getAttribute("aria-disabled")).toBe("true");
  expect(screen.queryByRole("option", { name: /Deleted sibling/ })).toBeNull();
  fireEvent.click(screen.getByTestId(PICKER));
}

async function selectOnBothSurfaces(name: string) {
  const triggers: HTMLElement[] = [];
  for (const surface of ["Task choices", "Subtask choices"]) {
    const trigger = within(screen.getByRole("region", { name: surface })).getByTestId(
      "agent-profile-selector",
    );
    fireEvent.click(trigger);
    const option = await screen.findByRole("option", { name: new RegExp(name) });
    expect(option.getAttribute("aria-disabled")).not.toBe("true");
    fireEvent.click(option);
    await waitFor(() => expect(trigger.textContent).toContain(name));
    triggers.push(trigger);
  }
  return triggers;
}

async function assertOptionsOnBothSurfaces(included: string[], excluded: string[]) {
  for (const surface of ["Task choices", "Subtask choices"]) {
    const trigger = within(screen.getByRole("region", { name: surface })).getByTestId(
      "agent-profile-selector",
    );
    fireEvent.click(trigger);
    await screen.findByRole("option", { name: new RegExp(included[0]) });
    const renderedChoices = screen.getAllByRole("option").map((option) => option.textContent);
    for (const name of included) expect(renderedChoices.join(" "), surface).toContain(name);
    for (const name of excluded)
      expect(screen.queryByRole("option", { name: new RegExp(name) })).toBeNull();
    fireEvent.click(trigger);
  }
}

describe("concrete profile save catalogue", () => {
  // @covers AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.1, .5
  it("keeps a live-deleted sibling unavailable after save acknowledgement", async () => {
    const h = mountEditor();
    expect(screen.getByTestId(PICKER).textContent).toContain("Deleted sibling");
    const { saving } = await h.beginSave();
    h.removeSibling();
    await assertUnavailable();
    expect((await h.finishSave(saving)).canLeave).toBe(true);
    expect(nestedProfile(h, h.targetId)?.name).toBe(SAVED_NAME);
    await assertUnavailable();
    expect(nestedProfile(h, h.siblingId)).toBeUndefined();
    expect(
      h.store.getState().agentProfiles.items.some((profile) => profile.id === h.siblingId),
    ).toBe(false);
  });

  // @covers AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.3
  it("publishes an ordinary accepted save", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    expect((await h.finishSave(saving)).canLeave).toBe(true);
    expect(nestedProfile(h, h.targetId)).toMatchObject({
      name: SAVED_NAME,
      updatedAt: h.ackRevision,
    });
    await waitFor(() => expect(h.coordinator.hasDirty).toBe(false));
    fireEvent.click(screen.getByTestId(PICKER));
    expect(await screen.findByRole("option", { name: new RegExp(SAVED_NAME) })).toBeTruthy();
    expect(await screen.findByRole("option", { name: /Deleted sibling/ })).toBeTruthy();
  });

  // @covers AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.4
  it("retains live deletion when PATCH is rejected", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.removeSibling();
    await act(async () => {
      h.pending.reject(new ApiError("Save refused", 500, null));
      expect((await saving).canLeave).toBe(false);
    });
    await assertUnavailable();
    expect(nestedProfile(h, h.siblingId)).toBeUndefined();
    expect(nestedProfile(h, h.targetId)?.name).toBe("Initial editor");
    expect((screen.getByTestId(PROFILE_NAME_INPUT) as HTMLInputElement).value).toBe(SAVED_NAME);
    expect(h.coordinator.hasDirty).toBe(true);
  });
});

describe("concrete save inventory preservation", () => {
  // @covers AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.2, .5
  it("preserves current sibling and independent inventory", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.receive(PROFILE_UPDATED, {
      ...h.sibling,
      name: "Latest sibling",
      enabled: false,
      updated_at: h.liveRevision,
    });
    const added = wireProfile(`added-${sequence}`, "Added sibling", h.ownerId, h.liveRevision);
    h.receive(PROFILE_CREATED, added);
    const loose = wireProfile(
      `loose-${sequence}`,
      "Independent choice",
      `absent-owner-${sequence}`,
      h.liveRevision,
    );
    h.receive(PROFILE_CREATED, loose);
    h.receive(PROFILE_UPDATED, {
      ...loose,
      name: INDEPENDENT_NAME,
      updated_at: h.liveRevision,
    });
    h.receive(PROFILE_CREATED, {
      ...wireProfile(
        `disabled-${sequence}`,
        "Disabled independent",
        `absent-disabled-${sequence}`,
        h.liveRevision,
      ),
      enabled: false,
    });
    h.receive(PROFILE_CREATED, {
      ...wireProfile(`office-${sequence}`, "Office excluded", h.ownerId, h.liveRevision),
      workspace_id: "office-workspace",
    });
    act(() =>
      h.store.setState((state) => ({
        settingsAgents: {
          items: [
            ...state.settingsAgents.items.map((owner) => ({
              ...owner,
              mcp_config_path: "latest-owner.json",
            })),
            {
              ...h.owner,
              id: `other-owner-${sequence}`,
              name: "other-agent",
              mcp_config_path: "other-latest.json",
              profiles: [],
            },
          ],
        },
      })),
    );
    const selected = await selectOnBothSurfaces(INDEPENDENT_NAME);
    const before = h.store.getState();
    const unrelatedProfiles = before.settingsAgents.items[0].profiles.filter(
      (profile) => profile.id !== h.targetId,
    );
    const unrelatedOptions = before.agentProfiles.items.filter(
      (profile) => profile.id !== h.targetId,
    );
    const otherOwner = before.settingsAgents.items[1];
    await h.finishSave(saving);
    const after = h.store.getState();
    expect(after.settingsAgents.items[0].mcp_config_path).toBe("latest-owner.json");
    expect(
      after.settingsAgents.items[0].profiles.filter((profile) => profile.id !== h.targetId),
    ).toEqual(unrelatedProfiles);
    expect(after.settingsAgents.items[1]).toBe(otherOwner);
    expect(after.agentProfiles.items.filter((profile) => profile.id !== h.targetId)).toEqual(
      unrelatedOptions,
    );
    expect(after.agentProfiles.version).toBe(before.agentProfiles.version);
    for (const trigger of selected) expect(trigger.textContent).toContain(INDEPENDENT_NAME);
    await assertOptionsOnBothSurfaces(
      [INDEPENDENT_NAME, "Added sibling", SAVED_NAME],
      ["Latest sibling", "Disabled independent", "Office excluded"],
    );
  });
});

describe("concrete save absence and reconciliation", () => {
  // @covers AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.2
  it.each(["target", "owner"] as const)("does not publish a removed %s", async (removed) => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.receive(PROFILE_DELETED, { ...h.target, updated_at: h.liveRevision });
    if (removed === "owner")
      act(() =>
        h.store.setState((state) => ({
          settingsAgents: {
            items: state.settingsAgents.items.filter((owner) => owner.id !== h.ownerId),
          },
        })),
      );
    await waitFor(() => expect(screen.queryByTestId(PROFILE_NAME_INPUT)).toBeNull());
    const before = h.store.getState();
    await h.finishSave(saving);
    expect(h.store.getState().settingsAgents.items).toEqual(before.settingsAgents.items);
    expect(h.store.getState().agentProfiles.items).toEqual(before.agentProfiles.items);
    expect(nestedProfile(h, h.targetId)).toBeUndefined();
  });

  // @covers AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.3
  it("retains newer revision when a late response is rejected", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.receive(PROFILE_UPDATED, {
      ...h.target,
      name: "Newer authoritative",
      updated_at: h.liveRevision,
    });
    expect(await screen.findByTestId("profile-external-change-alert")).toBeTruthy();
    await h.finishSave(saving);
    expect(nestedProfile(h, h.targetId)).toMatchObject({
      name: "Newer authoritative",
      updatedAt: h.liveRevision,
    });
    expect((screen.getByTestId(PROFILE_NAME_INPUT) as HTMLInputElement).value).toBe(SAVED_NAME);
    expect(screen.getByTestId("profile-external-change-alert")).toBeTruthy();
    await assertOptionsOnBothSurfaces(["Newer authoritative"], [SAVED_NAME]);
  });

  // @covers AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.3
  it("retains edits made during save", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    fireEvent.change(screen.getByTestId(PROFILE_NAME_INPUT), {
      target: { value: "Newer local draft" },
    });
    await h.finishSave(saving);
    expect(nestedProfile(h, h.targetId)?.name).toBe(SAVED_NAME);
    expect((screen.getByTestId(PROFILE_NAME_INPUT) as HTMLInputElement).value).toBe(
      "Newer local draft",
    );
    expect(h.coordinator.hasDirty).toBe(true);
    expect(screen.queryByTestId("profile-external-change-alert")).toBeNull();
  });
});
