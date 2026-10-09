import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { StoreApi } from "zustand";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { fetchJson } from "@/lib/api/client";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import type { AgentProfilePayload } from "@/lib/types/agent-profile";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { AgentProfilePicker } from "./agent-profile-picker";
import {
  useDynamicAgentProfileEditorState,
  type DynamicAgentProfileEditorState,
} from "./dynamic-agent-profile-editor-state";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "./settings-save-provider";

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

const SUITE_NAME = "dynamic profile standalone save concurrency";
const LIVE_CREATED_NAME = "Live created";
const START = "2026-10-06T10:00:00Z";
let responseRevision = "2026-10-06T10:01:00Z";
let liveRevision = "2026-10-06T10:02:00Z";
let fixtureRevision = 0;
const DYNAMIC_OWNER = "dynamic-owner";
const CONCRETE_OWNER = "concrete-owner";
const CREATED_PROFILE = "created-profile";
const TARGET = "dynamic-save-target";
const OTHER = "other-owner-profile";
const LOOSE = "absent-owner-profile";
const EDITED_NAME = "Saved routing";
const PROFILE_CREATED = "agent.profile.created";
const PROFILE_UPDATED = "agent.profile.updated";
const PROFILE_DELETED = "agent.profile.deleted";
const PICKER = "concurrent-save-picker";

function wireProfile(id: string, agentId: string, name: string): AgentProfilePayload {
  return {
    id,
    agent_id: agentId,
    agent_display_name: agentId === DYNAMIC_OWNER ? "Dynamic" : "Concrete",
    kind: agentId === DYNAMIC_OWNER ? "dynamic" : "concrete",
    name,
    model: "",
    allow_indexing: false,
    auto_approve: false,
    cli_flags: [],
    cli_passthrough: false,
    enabled: true,
    created_at: START,
    updated_at: START,
  };
}

function dynamicWire(): AgentProfilePayload {
  const policy = {
    retry: { enabled: false, max_retries: 0, initial_interval_seconds: 0 },
    wait_for_reset: { enabled: false, max_wait_seconds: 0 },
    on_exhausted: "skip" as const,
  };
  return {
    ...wireProfile(TARGET, DYNAMIC_OWNER, "Initial routing"),
    dynamic: {
      version: 1,
      candidates: [
        {
          position: 0,
          execution_profile_id: OTHER,
          enabled: true,
          policies: {
            version: 1,
            transient: policy,
            hard: policy,
            unclassified: { enabled: false, consecutive_failure_threshold: 0 },
          },
        },
      ],
    },
  };
}

function owner(id: string, name: string, ...profiles: AgentProfile[]): Agent {
  return {
    id,
    name,
    profiles,
    supports_mcp: false,
    inference_capable: true,
    created_at: START,
    updated_at: START,
  };
}

function deferredResponse() {
  let resolve!: (value: unknown) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<unknown>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

const pendingResponses: Array<ReturnType<typeof deferredResponse>> = [];

beforeEach(() => {
  vi.mocked(fetchJson).mockReset();
  fixtureRevision += 1;
  responseRevision = new Date(Date.UTC(2026, 9, 6, 10, fixtureRevision, 1)).toISOString();
  liveRevision = new Date(Date.UTC(2026, 9, 6, 10, fixtureRevision, 2)).toISOString();
});
afterEach(async () => {
  await act(async () => {
    for (const pending of pendingResponses.splice(0)) pending.resolve(dynamicWire());
  });
  cleanup();
  clearNavigationBlockerForTests();
});

function renderEditorProviders(dynamic: Agent, other: Agent, consumer: ReactNode) {
  render(
    <StateProvider
      initialState={{
        features: { ...defaultFeatureFlags, dynamicAgentRouting: true },
        settingsAgents: { items: [dynamic, other] },
        agentProfiles: {
          items: [
            toAgentProfileOption(dynamic, dynamic.profiles[0]),
            toAgentProfileOption(other, other.profiles[0]),
          ],
          version: 1,
        },
      }}
    >
      <TooltipProvider>
        <ToastProvider>
          <SettingsSaveProvider>{consumer}</SettingsSaveProvider>
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
}

function mountEditor(
  onDraftChange?: (patch: Pick<AgentProfile, "name" | "dynamic" | "enabled">) => void,
) {
  const dynamic = owner(DYNAMIC_OWNER, "dynamic", normalizeAgentProfile(dynamicWire()));
  const other = owner(
    CONCRETE_OWNER,
    "codex",
    normalizeAgentProfile(wireProfile(OTHER, CONCRETE_OWNER, "Candidate")),
  );
  const pending = deferredResponse();
  pendingResponses.push(pending);
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(_url: string, options?: Parameters<typeof fetchJson>[1]) => {
      if (options?.init?.method === "PATCH") return pending.promise as Promise<T>;
      return undefined as T;
    },
  );
  let store!: StoreApi<AppState>;
  let coordinator!: SettingsSaveCoordinator;
  let editor!: DynamicAgentProfileEditorState;

  function Editor({ agent, profile }: { agent: Agent; profile: AgentProfile }) {
    editor = useDynamicAgentProfileEditorState({ agent, profile, onDraftChange });
    return null;
  }

  function Consumer() {
    store = useAppStoreApi();
    coordinator = useSettingsSaveCoordinator();
    const agents = useAppStore((state) => state.settingsAgents.items);
    const options = useAppStore((state) => state.agentProfiles.items);
    const agent = agents.find((item) => item.id === dynamic.id);
    const profile = agent?.profiles.find((item) => item.id === TARGET);
    return (
      <>
        {agent && profile && <Editor agent={agent} profile={profile} />}
        <AgentProfilePicker
          profiles={options}
          value={LOOSE}
          onValueChange={() => undefined}
          testId={PICKER}
        />
      </>
    );
  }

  renderEditorProviders(dynamic, other, <Consumer />);

  function event(
    action: typeof PROFILE_CREATED | typeof PROFILE_UPDATED | typeof PROFILE_DELETED,
    profile: AgentProfilePayload,
  ) {
    const handlers = registerAgentsHandlers(store);
    const notification = {
      id: `live-${profile.id}`,
      type: "notification" as const,
      timestamp: profile.updated_at,
      payload: { profile: { ...profile, dangerously_skip_permissions: false, plan: "" } },
    };
    act(() => {
      if (action === PROFILE_CREATED) handlers[action]?.({ ...notification, action });
      else if (action === PROFILE_UPDATED) handlers[action]?.({ ...notification, action });
      else handlers[action]?.({ ...notification, action });
    });
  }

  async function beginSave() {
    act(() => editor.updateName(EDITED_NAME));
    await waitFor(() => expect(coordinator.hasDirty).toBe(true));
    let saving!: ReturnType<SettingsSaveCoordinator["saveAll"]>;
    act(() => {
      saving = coordinator.saveAll();
    });
    await waitFor(() => expect(patchCalls()).toHaveLength(1));
    return { saving };
  }

  async function finishSave(
    saving: ReturnType<SettingsSaveCoordinator["saveAll"]>,
    response = { ...dynamicWire(), name: EDITED_NAME, updated_at: responseRevision },
  ) {
    let result!: Awaited<typeof saving>;
    await act(async () => {
      pending.resolve(response);
      result = await saving;
    });
    return result;
  }

  return {
    get store() {
      return store;
    },
    get coordinator() {
      return coordinator;
    },
    get editor() {
      return editor;
    },
    pending,
    event,
    beginSave,
    finishSave,
  };
}

function patchCalls() {
  return vi.mocked(fetchJson).mock.calls.filter(([, options]) => options?.init?.method === "PATCH");
}

function profileState(harness: ReturnType<typeof mountEditor>, id: string) {
  const state = harness.store.getState();
  return {
    profile: state.settingsAgents.items
      .flatMap((agent) => agent.profiles)
      .find((profile) => profile.id === id),
    option: state.agentProfiles.items.find((option) => option.id === id),
  };
}

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.9
describe(SUITE_NAME, () => {
  it("retains a concurrent known-owner create", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.event(PROFILE_CREATED, {
      ...wireProfile(CREATED_PROFILE, CONCRETE_OWNER, LIVE_CREATED_NAME),
      updated_at: liveRevision,
    });
    expect(profileState(h, CREATED_PROFILE).profile?.name).toBe(LIVE_CREATED_NAME);
    await h.finishSave(saving);
    expect(profileState(h, CREATED_PROFILE)).toMatchObject({
      profile: { name: LIVE_CREATED_NAME, updatedAt: liveRevision },
      option: { label: "Concrete • Live created", enabled: true, updatedAt: liveRevision },
    });
  });

  it("retains a concurrent known-owner update", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.event(PROFILE_UPDATED, {
      ...wireProfile(OTHER, CONCRETE_OWNER, "Live renamed"),
      enabled: false,
      updated_at: liveRevision,
    });
    expect(profileState(h, OTHER).profile?.enabled).toBe(false);
    await h.finishSave(saving);
    expect(profileState(h, OTHER)).toMatchObject({
      profile: { name: "Live renamed", enabled: false, updatedAt: liveRevision },
      option: { label: "Concrete • Live renamed", enabled: false, updatedAt: liveRevision },
    });
  });

  it("retains a concurrent known-owner deletion", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.event(PROFILE_DELETED, {
      ...wireProfile(OTHER, CONCRETE_OWNER, "Candidate"),
      updated_at: liveRevision,
    });
    expect(profileState(h, OTHER)).toEqual({ profile: undefined, option: undefined });
    await h.finishSave(saving);
    expect(profileState(h, OTHER)).toEqual({ profile: undefined, option: undefined });
  });
});

describe(SUITE_NAME, () => {
  it("retains mixed represented and unrepresented options", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.event(PROFILE_CREATED, {
      ...wireProfile(LOOSE, "absent-owner", "Loose choice"),
      updated_at: liveRevision,
    });
    act(() =>
      h.store.getState().setAgentProfiles(
        h.store.getState().agentProfiles.items.map((option) =>
          option.id === OTHER
            ? {
                ...option,
                label: "Newer represented choice",
                enabled: false,
                updatedAt: liveRevision,
              }
            : option,
        ),
      ),
    );
    expect(profileState(h, LOOSE).option?.label).toBe("Concrete • Loose choice");
    await h.finishSave(saving);
    expect(profileState(h, LOOSE).option).toMatchObject({
      label: "Concrete • Loose choice",
      enabled: true,
      updatedAt: liveRevision,
    });
    expect(profileState(h, OTHER).option).toMatchObject({
      label: "Newer represented choice",
      enabled: false,
      updatedAt: liveRevision,
    });
  });

  it("retains the rendered unrepresented picker label", async () => {
    const h = mountEditor();
    h.event(PROFILE_CREATED, {
      ...wireProfile(LOOSE, "absent-owner", "Retained selection"),
      updated_at: liveRevision,
    });
    expect(screen.getByTestId(PICKER).textContent).toContain("Concrete • Retained selection");
    const { saving } = await h.beginSave();
    await h.finishSave(saving);
    expect(screen.getByTestId(PICKER).textContent).toContain("Concrete • Retained selection");
    expect(screen.getByTestId(PICKER).textContent).not.toContain("Unavailable");
  });

  it("preserves a newer targeted profile revision", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.event(PROFILE_UPDATED, {
      ...dynamicWire(),
      name: "External newer routing",
      updated_at: liveRevision,
    });
    expect(h.editor.hasExternalConflict).toBe(true);
    await h.finishSave(saving);
    expect(profileState(h, TARGET)).toMatchObject({
      profile: { name: "External newer routing", updatedAt: liveRevision },
      option: { label: "Dynamic • External newer routing", updatedAt: liveRevision },
    });
    expect(h.editor.name).toBe(EDITED_NAME);
    expect(h.editor.hasExternalConflict).toBe(true);
  });
});

describe(SUITE_NAME, () => {
  it("does not insert an absent target", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.event(PROFILE_DELETED, { ...dynamicWire(), updated_at: liveRevision });
    await h.finishSave(saving);
    expect(profileState(h, TARGET)).toEqual({ profile: undefined, option: undefined });
  });

  it("does not insert an absent owner", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    act(() =>
      h.store
        .getState()
        .setSettingsAgents(
          h.store.getState().settingsAgents.items.filter((agent) => agent.id !== DYNAMIC_OWNER),
        ),
    );
    await h.finishSave(saving);
    expect(h.store.getState().settingsAgents.items.map((agent) => agent.id)).toEqual([
      CONCRETE_OWNER,
    ]);
    expect(profileState(h, TARGET).profile).toBeUndefined();
  });

  it("accepts an ordinary current response and request", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    const [[url, options]] = patchCalls();
    expect(new URL(url).pathname).toBe(`/api/v1/agent-profiles/${TARGET}`);
    expect(JSON.parse(String(options?.init?.body))).toEqual({
      name: EDITED_NAME,
      enabled: true,
      dynamic: dynamicWire().dynamic,
    });
    const result = await h.finishSave(saving);
    expect(result.canLeave).toBe(true);
    expect(profileState(h, TARGET)).toMatchObject({
      profile: { name: EDITED_NAME, updatedAt: responseRevision },
      option: { label: `Dynamic • ${EDITED_NAME}`, updatedAt: responseRevision },
    });
    expect(h.coordinator.hasDirty).toBe(false);
    expect(h.editor.hasExternalConflict).toBe(false);
    expect(screen.getByText("Dynamic profile saved.")).toBeTruthy();
  });
});

describe(SUITE_NAME, () => {
  it("accepts its own websocket acknowledgement", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    const ack = { ...dynamicWire(), name: EDITED_NAME, updated_at: responseRevision };
    h.event(PROFILE_UPDATED, ack);
    expect(profileState(h, TARGET).profile?.updatedAt).toBe(responseRevision);
    expect(h.coordinator.hasDirty).toBe(false);
    expect(h.editor.hasExternalConflict).toBe(false);
    await h.finishSave(saving, ack);
    expect(h.coordinator.hasDirty).toBe(false);
    expect(h.editor.hasExternalConflict).toBe(false);
    expect(
      h.store.getState().agentProfiles.items.filter((option) => option.id === TARGET),
    ).toHaveLength(1);
  });

  it("preserves state on a failed response", async () => {
    const h = mountEditor();
    const { saving } = await h.beginSave();
    h.event(PROFILE_CREATED, {
      ...wireProfile("created-before-failure", CONCRETE_OWNER, "Live during failure"),
      updated_at: liveRevision,
    });
    const agents = h.store.getState().settingsAgents.items;
    const options = h.store.getState().agentProfiles.items;
    await act(async () => {
      h.pending.reject(new Error("rejected patch"));
      await saving;
    });
    expect(h.store.getState().settingsAgents.items).toBe(agents);
    expect(h.store.getState().agentProfiles.items).toBe(options);
    expect(h.editor.name).toBe(EDITED_NAME);
    expect(h.coordinator.hasDirty).toBe(true);
    expect(h.coordinator.contributorStates[0].invalid).toBe(false);
    expect(screen.getByText("Failed to save profile")).toBeTruthy();
    expect(screen.queryByText("Dynamic profile saved.")).toBeNull();
  });

  it("blocks an invalid policy through the coordinator", async () => {
    const h = mountEditor();
    act(() =>
      h.editor.updateCandidatePolicy(0, "transient", {
        retry: { enabled: true, maxRetries: 0, initialIntervalSeconds: 0 },
      }),
    );
    await waitFor(() => expect(h.coordinator.contributorStates[0]?.invalid).toBe(true));
    const agents = h.store.getState().settingsAgents.items;
    let result!: Awaited<ReturnType<SettingsSaveCoordinator["saveAll"]>>;
    await act(async () => {
      result = await h.coordinator.saveAll();
    });
    expect(result.canLeave).toBe(false);
    expect(patchCalls()).toHaveLength(0);
    expect(h.store.getState().settingsAgents.items).toBe(agents);
  });
});

describe(SUITE_NAME, () => {
  it("keeps embedded drafts parent-owned", async () => {
    const onDraftChange = vi.fn();
    const h = mountEditor(onDraftChange);
    const agents = h.store.getState().settingsAgents.items;
    act(() => h.editor.updateName(EDITED_NAME));
    expect(onDraftChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        name: EDITED_NAME,
        enabled: true,
        dynamic: expect.objectContaining({ version: 1 }),
      }),
    );
    expect(h.editor.standalone).toBe(false);
    await act(async () => {
      await h.coordinator.saveAll();
    });
    expect(patchCalls()).toHaveLength(0);
    expect(h.store.getState().settingsAgents.items).toBe(agents);
  });
});
