import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { AgentProfilePicker } from "@/components/settings/agent-profile-picker";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { fetchJson } from "@/lib/api/client";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type { AppState } from "@/lib/state/store";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AgentProfilePayload } from "@/lib/types/agent-profile";
import type { AgentProfilePayload as WsAgentProfilePayload } from "@/lib/types/backend";
import type { Agent, AvailableAgent } from "@/lib/types/http";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import AgentSetupPage from "./page";
import type { DraftProfile } from "./agent-save-helpers";

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

const CREATE_ROUTE = "/settings/agents/claude-code?mode=create";
const TARGET_OWNER = "creation-owner";
const OTHER_OWNER = "independent-owner";
const RECEIVED_ID = "received-profile";
const ACCEPTED_ID = "accepted-profile";
const DRAFT_NAME = "Created reviewer";
const MODEL = "model-reviewed";
const PICKER = "creation-catalogue-picker";
let fixtureNumber = 0;
let timestamp = "";

function profileWire(
  id: string,
  owner: string,
  name: string,
): AgentProfilePayload & WsAgentProfilePayload {
  return {
    id,
    agent_id: owner,
    agent_display_name: owner === TARGET_OWNER ? "Claude Code" : "Codex",
    name,
    kind: "concrete",
    model: MODEL,
    allow_indexing: false,
    auto_approve: false,
    dangerously_skip_permissions: false,
    plan: "",
    cli_flags: [],
    cli_passthrough: false,
    enabled: true,
    created_at: timestamp,
    updated_at: timestamp,
  };
}

function owner(id: string, name: string, profileName: string): Agent {
  return {
    id,
    name,
    supports_mcp: true,
    mcp_config_path: "",
    inference_capable: true,
    profiles: [normalizeAgentProfile(profileWire(`${id}-existing`, id, profileName))],
    created_at: timestamp,
    updated_at: timestamp,
  };
}

function availableAgent(): AvailableAgent {
  return {
    name: "claude-code",
    display_name: "Claude Code",
    supports_mcp: true,
    installation_paths: [],
    available: true,
    capabilities: {
      supports_session_resume: false,
      supports_shell: false,
      supports_workspace_only: false,
    },
    model_config: {
      default_model: MODEL,
      available_models: [{ id: MODEL, name: "Reviewed model" }],
      supports_dynamic_models: false,
    },
    permission_settings: {},
    updated_at: timestamp,
  };
}

function deferredResponse(cleanupValue: unknown) {
  let resolve!: (value: unknown) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<unknown>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject, cleanupValue };
}

type SaveResult = Awaited<ReturnType<SettingsSaveCoordinator["saveAll"]>>;
const pendingResponses: ReturnType<typeof deferredResponse>[] = [];
const pendingSaves: Promise<SaveResult>[] = [];

beforeEach(() => {
  fixtureNumber += 1;
  timestamp = new Date(Date.UTC(2026, 9, 9, 12, fixtureNumber)).toISOString();
  vi.mocked(fetchJson).mockReset();
  window.history.replaceState({}, "", CREATE_ROUTE);
});

afterEach(async () => {
  await act(async () => {
    for (const pending of pendingResponses.splice(0)) pending.resolve(pending.cleanupValue);
    await Promise.all(pendingSaves.splice(0));
  });
  cleanup();
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", "/");
});

function mountCreation(newOwner = false, failMcp = false) {
  const target = owner(TARGET_OWNER, "claude-code", "Existing target");
  const independent = owner(OTHER_OWNER, "codex", "Existing independent");
  const agents = newOwner ? [independent] : [target, independent];
  const accepted = profileWire(ACCEPTED_ID, TARGET_OWNER, DRAFT_NAME);
  const response = newOwner ? { ...target, profiles: [accepted] } : accepted;
  const pending = deferredResponse(response);
  pendingResponses.push(pending);
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(url: string, options?: Parameters<typeof fetchJson>[1]) => {
      const path = new URL(url).pathname;
      const method = options?.init?.method ?? "GET";
      requests.push({ path, method, body: options?.init?.body });
      if (method === "POST" && path.endsWith("/mcp-config")) {
        if (failMcp) throw new Error("Accepted creation MCP failure");
        return { profile_id: ACCEPTED_ID, enabled: true, servers: {} } as T;
      }
      if (method === "POST") return pending.promise as Promise<T>;
      if (method === "GET" && path.endsWith("/mcp-config")) {
        return { profile_id: ACCEPTED_ID, enabled: false, servers: {} } as T;
      }
      throw new Error(`Unexpected transport request ${method} ${path}`);
    },
  );
  let store!: StoreApi<AppState>;
  let coordinator!: SettingsSaveCoordinator;
  function Consumer() {
    store = useAppStoreApi();
    coordinator = useSettingsSaveCoordinator();
    const options = useAppStore((state) => state.agentProfiles.items);
    const [selected, setSelected] = useState("");
    return (
      <>
        <AgentSetupPage />
        <AgentProfilePicker
          profiles={options}
          value={selected}
          onValueChange={setSelected}
          testId={PICKER}
        />
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
            id: "creation-admin",
            email: "admin@example.test",
            display_name: "Administrator",
            role: "admin",
            status: "active",
          },
        },
        settingsAgents: { items: agents },
        agentProfiles: {
          orderByAgent: {},
          items: agents.flatMap((agent) =>
            agent.profiles.map((p) => toAgentProfileOption(agent, p)),
          ),
          version: 1,
        },
        agentDiscovery: { items: [availableAgent()], loaded: true, loading: false },
        availableAgents: { items: [availableAgent()], tools: [], loaded: true, loading: false },
        secrets: { items: [], loaded: true, loading: false },
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
  return { pending, requests, getStore: () => store, getCoordinator: () => coordinator };
}

type Creation = ReturnType<typeof mountCreation>;

async function submitCreation(context: Creation) {
  fireEvent.change(screen.getByTestId("profile-name-input"), { target: { value: DRAFT_NAME } });
  await waitFor(() => expect(context.getCoordinator().hasDirty).toBe(true));
  let save!: Promise<SaveResult>;
  act(() => {
    save = context.getCoordinator().saveAll();
    pendingSaves.push(save);
  });
  await waitFor(() =>
    expect(context.requests.some((request) => request.method === "POST")).toBe(true),
  );
  return { save };
}

async function receiveAndSelectIndependent(context: Creation) {
  act(() => {
    const handleCreated = registerAgentsHandlers(context.getStore())["agent.profile.created"]!;
    handleCreated({
      id: `received-${fixtureNumber}`,
      type: "notification",
      action: "agent.profile.created",
      timestamp,
      payload: { profile: profileWire(RECEIVED_ID, OTHER_OWNER, "Arrived profile") },
    });
  });
  fireEvent.click(screen.getByTestId(PICKER));
  const option = screen.getByRole("option", { name: /Arrived profile/ });
  expect(option.getAttribute("aria-disabled")).toBe("false");
  fireEvent.click(option);
  await waitFor(() => expect(screen.getByTestId(PICKER).textContent).toContain("Arrived profile"));
}

function assertIndependentRetained(context: Creation) {
  const state = context.getStore().getState();
  const other = state.settingsAgents.items.find((agent) => agent.id === OTHER_OWNER)!;
  expect.soft(other.profiles.map((profile) => profile.id)).toContain(RECEIVED_ID);
  expect.soft(screen.getByTestId(PICKER).textContent).toContain("Arrived profile");
  expect.soft(state.agentProfiles.items.map((profile) => profile.id)).toContain(RECEIVED_ID);
  expect.soft(screen.getByTestId(PICKER).textContent).not.toContain("Unavailable");
  fireEvent.click(screen.getByTestId(PICKER));
  const option = screen.queryByRole("option", { name: /Arrived profile/ });
  expect.soft(option?.getAttribute("aria-disabled")).toBe("false");
  if (option) fireEvent.click(option);
}

function editMcpDraft() {
  fireEvent.click(screen.getByTestId("mcp-enabled"));
  fireEvent.change(screen.getByTestId(/^mcp-servers-/), {
    target: { value: '{"mcpServers":{"review-tools":{"command":"review-server"}}}' },
  });
}

function assertPartialResult(context: Creation, result: SaveResult) {
  const target = context
    .getStore()
    .getState()
    .settingsAgents.items.find((agent) => agent.id === TARGET_OWNER)!;
  const profile = target.profiles.find((item) => item.id === ACCEPTED_ID) as DraftProfile;
  expect(profile).toMatchObject({
    id: ACCEPTED_ID,
    name: DRAFT_NAME,
    mcp_config: { enabled: true, dirty: true },
  });
  expect(profile.mcp_config?.servers).toContain("review-tools");
  expect(result.canLeave).toBe(false);
  expect(result.failedIds.size).toBe(1);
  expect(context.getCoordinator()).toMatchObject({ status: "error", errorKind: "save" });
  expect(screen.getAllByText("Accepted creation MCP failure").length).toBeGreaterThan(0);
  const mcp = context.requests.find(
    (request) => request.path.endsWith("/mcp-config") && request.method === "POST",
  )!;
  expect(mcp.path).toBe(`/api/v1/agent-profiles/${ACCEPTED_ID}/mcp-config`);
  expect(JSON.parse(mcp.body as string)).toMatchObject({
    enabled: true,
    mcpServers: { "review-tools": { command: "review-server" } },
  });
}

async function completeCreation(context: Creation, save: Promise<SaveResult>) {
  let result!: SaveResult;
  await act(async () => {
    context.pending.resolve(context.pending.cleanupValue);
    result = await save;
  });
  return result;
}

describe("normal creation catalogue publication", () => {
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.1
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.2
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.6
  it("retains a live different-owner profile after accepted profile creation", async () => {
    const context = mountCreation();
    const { save } = await submitCreation(context);
    expect(context.requests[0].path).toBe(`/api/v1/agents/${TARGET_OWNER}/profiles`);
    await receiveAndSelectIndependent(context);
    const result = await completeCreation(context, save);
    expect(result.failedIds.size).toBe(0);
    assertIndependentRetained(context);
  });

  // @covers AC-AGENTS-CREATION-CATALOGUE-001.2
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.6
  it("accepts profile creation without an intervening event", async () => {
    const context = mountCreation();
    const { save } = await submitCreation(context);
    const result = await completeCreation(context, save);
    expect(result.failedIds.size).toBe(0);
    const state = context.getStore().getState();
    const target = state.settingsAgents.items.find((agent) => agent.id === TARGET_OWNER)!;
    expect(target.profiles.map((profile) => profile.id)).toEqual([
      ACCEPTED_ID,
      `${TARGET_OWNER}-existing`,
    ]);
    expect(target.profiles.find((profile) => profile.id === ACCEPTED_ID)).toMatchObject({
      name: DRAFT_NAME,
      model: MODEL,
    });
    expect(state.agentProfiles.items.filter((profile) => profile.id === ACCEPTED_ID)).toHaveLength(
      1,
    );
    expect(JSON.parse(context.requests[0].body as string)).toMatchObject({
      name: DRAFT_NAME,
      model: MODEL,
    });
    await waitFor(() => expect(window.location.pathname).toBe("/settings/agents/claude-code"));
    expect(context.getCoordinator().hasDirty).toBe(false);
  });

  // @covers AC-AGENTS-CREATION-CATALOGUE-001.3
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.6
  it("rejects creation while preserving the live choice and newer draft", async () => {
    const context = mountCreation();
    const { save } = await submitCreation(context);
    await receiveAndSelectIndependent(context);
    fireEvent.change(screen.getByTestId("profile-name-input"), {
      target: { value: "Newer unsaved name" },
    });
    let result!: SaveResult;
    await act(async () => {
      context.pending.reject(new Error("Creation refused"));
      result = await save;
    });
    assertIndependentRetained(context);
    expect(result.canLeave).toBe(false);
    expect(result.failedIds.size).toBe(1);
    expect(context.getCoordinator()).toMatchObject({ hasDirty: true, status: "error" });
    expect((screen.getByTestId("profile-name-input") as HTMLInputElement).value).toBe(
      "Newer unsaved name",
    );
    expect(window.location.pathname + window.location.search).toBe(CREATE_ROUTE);
    expect(
      context
        .getStore()
        .getState()
        .agentProfiles.items.some((profile) => profile.id === ACCEPTED_ID),
    ).toBe(false);
  });
});

describe("creation partial results and new owners", () => {
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.1
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.4
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.6
  it("publishes partial accepted profile creation over the current catalogue", async () => {
    const context = mountCreation(false, true);
    editMcpDraft();
    const { save } = await submitCreation(context);
    await receiveAndSelectIndependent(context);
    const result = await completeCreation(context, save);
    assertIndependentRetained(context);
    assertPartialResult(context, result);
    expect(window.location.pathname + window.location.search).toBe(CREATE_ROUTE);
    expect(context.getCoordinator().hasDirty).toBe(true);
    expect(
      context
        .getStore()
        .getState()
        .settingsAgents.items.find((agent) => agent.id === TARGET_OWNER)!
        .profiles.map((p) => p.id),
    ).toEqual([ACCEPTED_ID, `${TARGET_OWNER}-existing`]);
  });

  // @covers AC-AGENTS-CREATION-CATALOGUE-001.5
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.6
  it("publishes a newly created agent over the current catalogue", async () => {
    const context = mountCreation(true);
    const { save } = await submitCreation(context);
    expect(context.requests[0].path).toBe("/api/v1/agents");
    await receiveAndSelectIndependent(context);
    const result = await completeCreation(context, save);
    expect(result.failedIds.size).toBe(0);
    assertIndependentRetained(context);
    const state = context.getStore().getState();
    expect(state.settingsAgents.items.map((agent) => agent.id)).toEqual([
      OTHER_OWNER,
      TARGET_OWNER,
    ]);
    expect(state.settingsAgents.items[1].profiles).toHaveLength(1);
    expect(state.settingsAgents.items[1].profiles[0]).toMatchObject({
      id: ACCEPTED_ID,
      agentId: TARGET_OWNER,
      name: DRAFT_NAME,
      model: MODEL,
    });
    expect(state.agentProfiles.items.filter((profile) => profile.id === ACCEPTED_ID)).toHaveLength(
      1,
    );
    await waitFor(() => expect(window.location.pathname).toBe("/settings/agents/claude-code"));
  });

  // @covers AC-AGENTS-CREATION-CATALOGUE-001.5
  // @covers AC-AGENTS-CREATION-CATALOGUE-001.6
  it("publishes a new agent partial MCP result without dropping a live choice", async () => {
    const context = mountCreation(true, true);
    editMcpDraft();
    const { save } = await submitCreation(context);
    await receiveAndSelectIndependent(context);
    const result = await completeCreation(context, save);
    assertIndependentRetained(context);
    assertPartialResult(context, result);
    expect(
      context
        .getStore()
        .getState()
        .settingsAgents.items.map((agent) => agent.id),
    ).toEqual([OTHER_OWNER, TARGET_OWNER]);
    await waitFor(() => expect(window.location.pathname).toBe("/settings/agents/claude-code"));
  });
});
