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
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AppState } from "@/lib/state/store";
import type { AgentProfilePayload } from "@/lib/types/agent-profile";
import type { AgentProfilePayload as WsProfile } from "@/lib/types/backend";
import type { Agent, AvailableAgent } from "@/lib/types/http";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import AgentSetupPage from "./page";
import type { DraftProfile } from "./agent-save-helpers";

vi.mock("@/lib/api/client", async (original) => ({
  ...(await original<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

const OWNER = "target-creation-owner";
const EXISTING = "target-original-profile";
const SIBLING = "target-arrived-profile";
const ACCEPTED = "target-accepted-profile";
const ROUTE = "/settings/agents/claude-code?mode=create";
const PICKER = "target-creation-picker";
const NAME = "Additional reviewer";
const MODEL = "review-model";
const LIVE_MODEL = "live-model";
const NAME_INPUT = "profile-name-input";
let sequence = 0;
let instant = "";
type SaveResult = Awaited<ReturnType<SettingsSaveCoordinator["saveAll"]>>;
const saves: Promise<SaveResult>[] = [];
const finishers: Array<() => void> = [];

function wire(id: string, name: string, overrides: Partial<AgentProfilePayload> = {}) {
  return {
    id,
    agent_id: OWNER,
    agent_display_name: "Claude Code",
    kind: "concrete",
    name,
    model: MODEL,
    allow_indexing: false,
    auto_approve: false,
    dangerously_skip_permissions: false,
    plan: "",
    cli_flags: [],
    cli_passthrough: false,
    enabled: true,
    created_at: instant,
    updated_at: instant,
    ...overrides,
  } satisfies AgentProfilePayload & WsProfile;
}

function configuredOwner(): Agent {
  return {
    id: OWNER,
    name: "claude-code",
    supports_mcp: true,
    mcp_config_path: "",
    inference_capable: true,
    profiles: [normalizeAgentProfile(wire(EXISTING, "Original profile"))],
    created_at: instant,
    updated_at: instant,
  };
}

function discovery(): AvailableAgent {
  return {
    name: "claude-code",
    display_name: "Claude Code",
    available: true,
    supports_mcp: true,
    installation_paths: [],
    capabilities: {
      supports_session_resume: false,
      supports_shell: false,
      supports_workspace_only: false,
    },
    model_config: {
      default_model: MODEL,
      available_models: [
        { id: MODEL, name: "Review model" },
        { id: LIVE_MODEL, name: "Live model" },
      ],
      supports_dynamic_models: false,
    },
    permission_settings: {},
    updated_at: instant,
  };
}

function acceptedPatch(raw: unknown) {
  const body = JSON.parse(String(raw)) as Record<string, unknown>;
  return wire(ACCEPTED, String(body.name ?? NAME), {
    ...body,
    updated_at: new Date(Date.parse(instant) + 200).toISOString(),
  });
}

function transport() {
  let accept!: (response: unknown) => void;
  let reject!: (error: Error) => void;
  const pending = new Promise<unknown>((resolve, fail) => {
    accept = resolve;
    reject = fail;
  });
  finishers.push(() => accept(wire(ACCEPTED, NAME)));
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const state = { failMcp: false };
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(url: string, options?: Parameters<typeof fetchJson>[1]) => {
      const path = new URL(url).pathname;
      const method = options?.init?.method ?? "GET";
      requests.push({ path, method, body: options?.init?.body });
      if (path.endsWith("/mcp-config")) {
        if (method === "POST" && state.failMcp) throw new Error("Target MCP save refused");
        return { profile_id: ACCEPTED, enabled: method === "POST", servers: {} } as T;
      }
      if (path === `/api/v1/agent-profiles/${ACCEPTED}` && method === "PATCH")
        return acceptedPatch(options?.init?.body) as T;
      if (path === `/api/v1/agents/${OWNER}/profiles` && method === "POST")
        return pending as Promise<T>;
      throw new Error(`Unexpected target transport: ${method} ${path}`);
    },
  );
  return { accept, reject, requests, state };
}

function mount() {
  const owner = configuredOwner();
  const remote = transport();
  let store!: StoreApi<AppState>;
  let coordinator!: SettingsSaveCoordinator;
  function Subject() {
    store = useAppStoreApi();
    coordinator = useSettingsSaveCoordinator();
    const profiles = useAppStore((state) => state.agentProfiles.items);
    const [selected, setSelected] = useState("");
    return (
      <>
        <AgentSetupPage />
        <AgentProfilePicker
          profiles={profiles}
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
            id: "target-admin",
            email: "target@example.test",
            display_name: "Admin",
            role: "admin",
            status: "active",
          },
        },
        settingsAgents: { items: [owner] },
        agentProfiles: {
          orderByAgent: {},
          items: owner.profiles.map((profile) => toAgentProfileOption(owner, profile)),
          version: 1,
        },
        agentDiscovery: { items: [discovery()], loaded: true, loading: false },
        availableAgents: { items: [discovery()], tools: [], loaded: true, loading: false },
        secrets: { items: [], loaded: true, loading: false },
      }}
    >
      <TooltipProvider>
        <ToastProvider>
          <SettingsSaveProvider>
            <Subject />
          </SettingsSaveProvider>
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
  return { ...remote, store: () => store, coordinator: () => coordinator };
}

type Context = ReturnType<typeof mount>;

beforeEach(() => {
  sequence += 1;
  instant = new Date(Date.UTC(2026, 9, 10, 12, sequence)).toISOString();
  vi.mocked(fetchJson).mockReset();
  window.history.replaceState({}, "", ROUTE);
});

afterEach(async () => {
  await act(async () => {
    finishers.splice(0).forEach((finish) => finish());
    await Promise.all(saves.splice(0));
  });
  cleanup();
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", "/");
});

async function submit(context: Context) {
  fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: NAME } });
  await waitFor(() => expect(context.coordinator().hasDirty).toBe(true));
  let save!: Promise<SaveResult>;
  act(() => {
    save = context.coordinator().saveAll();
    saves.push(save);
  });
  await waitFor(() =>
    expect(
      context.requests.some(
        (request) =>
          request.path === `/api/v1/agents/${OWNER}/profiles` && request.method === "POST",
      ),
    ).toBe(true),
  );
  return { save };
}

function receive(context: Context, profile = wire(SIBLING, "Live sibling")) {
  act(() =>
    registerAgentsHandlers(context.store())["agent.profile.created"]!({
      id: `target-event-${sequence}-${profile.id}`,
      type: "notification",
      action: "agent.profile.created",
      timestamp: profile.updated_at,
      payload: { profile },
    }),
  );
}

async function selectSibling(context: Context) {
  receive(context);
  fireEvent.click(screen.getByTestId(PICKER));
  const option = screen.getByRole("option", { name: /Live sibling/ });
  expect(option.getAttribute("aria-disabled")).toBe("false");
  fireEvent.click(option);
  await waitFor(() => expect(screen.getByTestId(PICKER).textContent).toContain("Live sibling"));
}

function assertSibling(context: Context) {
  const state = context.store().getState();
  const owner = state.settingsAgents.items.find((item) => item.id === OWNER)!;
  expect.soft(owner.profiles.map((profile) => profile.id)).toContain(SIBLING);
  expect.soft(state.agentProfiles.items.map((profile) => profile.id)).toContain(SIBLING);
  expect.soft(screen.getByTestId(PICKER).textContent).toContain("Live sibling");
  expect.soft(screen.getByTestId(PICKER).textContent).not.toContain("Unavailable");
  fireEvent.click(screen.getByTestId(PICKER));
  const choice = screen.queryByRole("option", { name: /Live sibling/ });
  expect.soft(choice?.getAttribute("aria-disabled")).toBe("false");
  if (choice) fireEvent.click(choice);
}

async function acknowledge(context: Context, save: Promise<SaveResult>) {
  let result!: SaveResult;
  await act(async () => {
    context.accept(wire(ACCEPTED, NAME));
    result = await save;
  });
  return result;
}

describe("existing-owner creation publication", () => {
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.1
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.2
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.6
  it("retains a selected same-owner sibling after creation ACK", async () => {
    const context = mount();
    const { save } = await submit(context);
    await selectSibling(context);
    expect((await acknowledge(context, save)).failedIds.size).toBe(0);
    assertSibling(context);
  });

  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.3
  it("accepts additional-profile creation without a live event", async () => {
    const context = mount();
    const { save } = await submit(context);
    expect((await acknowledge(context, save)).failedIds.size).toBe(0);
    const state = context.store().getState();
    expect(state.settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      ACCEPTED,
      EXISTING,
    ]);
    expect(
      state.settingsAgents.items[0].profiles.find((profile) => profile.id === ACCEPTED),
    ).toMatchObject({
      id: ACCEPTED,
      agentId: OWNER,
      name: NAME,
      model: MODEL,
    });
    expect(state.agentProfiles.items.filter((profile) => profile.id === ACCEPTED)).toHaveLength(1);
    expect(JSON.parse(context.requests[0].body as string)).toMatchObject({
      name: NAME,
      model: MODEL,
      cli_passthrough: false,
    });
    await waitFor(() => expect(window.location.pathname).toBe("/settings/agents/claude-code"));
    expect(context.coordinator().hasDirty).toBe(false);
  });

  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.4
  it("rejected creation retains live sibling and newer draft", async () => {
    const context = mount();
    const { save } = await submit(context);
    await selectSibling(context);
    fireEvent.change(screen.getByTestId(NAME_INPUT), {
      target: { value: "Unsaved newer name" },
    });
    let result!: SaveResult;
    await act(async () => {
      context.reject(new Error("Target creation refused"));
      result = await save;
    });
    assertSibling(context);
    expect(result.canLeave).toBe(false);
    expect(result.failedIds.size).toBe(1);
    expect(context.coordinator()).toMatchObject({ status: "error", hasDirty: true });
    expect((screen.getByTestId(NAME_INPUT) as HTMLInputElement).value).toBe("Unsaved newer name");
    expect(window.location.pathname + window.location.search).toBe(ROUTE);
    expect(
      context
        .store()
        .getState()
        .agentProfiles.items.some((profile) => profile.id === ACCEPTED),
    ).toBe(false);
  });
});

describe("current target metadata", () => {
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.2
  it("creation retains current sibling and owner metadata", async () => {
    const context = mount();
    const { save } = await submit(context);
    await selectSibling(context);
    const latest = wire(EXISTING, "Updated original", {
      model: "updated-model",
      enabled: false,
      config_options: { reasoning: "high" },
      updated_at: new Date(Date.parse(instant) + 100).toISOString(),
    });
    receive(context, latest);
    act(() =>
      context.store().setState((state) => ({
        settingsAgents: {
          items: state.settingsAgents.items.map((owner) => ({
            ...owner,
            supports_mcp: false,
            inference_capable: false,
            mcp_config_path: "live-path",
          })),
        },
      })),
    );
    expect((await acknowledge(context, save)).failedIds.size).toBe(0);
    assertSibling(context);
    const state = context.store().getState();
    expect(state.settingsAgents.items[0]).toMatchObject({
      supports_mcp: false,
      inference_capable: false,
      mcp_config_path: "live-path",
    });
    expect(
      state.settingsAgents.items[0].profiles.find((profile) => profile.id === EXISTING),
    ).toMatchObject({
      name: "Updated original",
      model: "updated-model",
      enabled: false,
      configOptions: { reasoning: "high" },
      updatedAt: latest.updated_at,
    });
    expect(state.agentProfiles.items.find((profile) => profile.id === EXISTING)).toMatchObject({
      model: "updated-model",
      enabled: false,
    });
  });
});

describe("accepted partial creation retry", () => {
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.5
  it("partial MCP failure retains live siblings and remaps the accepted draft for retry", async () => {
    const context = mount();
    context.state.failMcp = true;
    fireEvent.click(screen.getByTestId("mcp-enabled"));
    fireEvent.change(screen.getByTestId(/^mcp-servers-/), {
      target: { value: '{"mcpServers":{"target-tool":{"command":"review-server"}}}' },
    });
    const { save } = await submit(context);
    await selectSibling(context);
    const result = await acknowledge(context, save);
    assertSibling(context);
    expect(result.failedIds.size).toBe(1);
    expect(result.canLeave).toBe(false);
    expect(context.coordinator()).toMatchObject({ status: "error", hasDirty: true });
    expect(window.location.pathname + window.location.search).toBe(ROUTE);
    const accepted = context
      .store()
      .getState()
      .settingsAgents.items[0].profiles.find((profile) => profile.id === ACCEPTED) as DraftProfile;
    expect(accepted).toMatchObject({ id: ACCEPTED, mcp_config: { enabled: true, dirty: true } });
    expect(accepted.mcp_config?.servers).toContain("target-tool");
    expect(screen.getAllByText("Target MCP save refused").length).toBeGreaterThan(0);
    context.state.failMcp = false;
    let retry!: SaveResult;
    await act(async () => {
      retry = await context.coordinator().saveAll();
    });
    expect(retry.failedIds.size).toBe(0);
    const state = context.store().getState();
    expect(
      state.settingsAgents.items[0].profiles.filter((profile) => profile.id === ACCEPTED),
    ).toHaveLength(1);
    expect(
      (
        state.settingsAgents.items[0].profiles.find(
          (profile) => profile.id === ACCEPTED,
        ) as DraftProfile
      ).mcp_config,
    ).toBeUndefined();
    expect(
      context.requests.filter(
        (request) =>
          request.path === `/api/v1/agents/${OWNER}/profiles` && request.method === "POST",
      ),
    ).toHaveLength(1);
    expect(
      context.requests.filter(
        (request) =>
          request.path === `/api/v1/agent-profiles/${ACCEPTED}/mcp-config` &&
          request.method === "POST",
      ),
    ).toHaveLength(2);
    expect(state.settingsAgents.items[0].profiles.map((profile) => profile.id)).toContain(SIBLING);
    await waitFor(() => expect(window.location.pathname).toBe("/settings/agents/claude-code"));
    expect(context.coordinator().hasDirty).toBe(false);
  });
});

describe("partial creation retry with a newer accepted copy", () => {
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.2
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.5
  it.each([false, true])(
    "retains live metadata on retry with in-flight edit=%s",
    async (edited) => {
      const context = mount();
      context.state.failMcp = true;
      fireEvent.click(screen.getByTestId("mcp-enabled"));
      const { save } = await submit(context);
      receive(
        context,
        wire(ACCEPTED, "Live accepted name", {
          model: LIVE_MODEL,
          updated_at: new Date(Date.parse(instant) + 100).toISOString(),
        }),
      );
      if (edited) {
        fireEvent.change(screen.getByTestId(NAME_INPUT), {
          target: { value: "Genuine draft edit" },
        });
      }
      const failed = await acknowledge(context, save);
      expect(failed.failedIds.size).toBe(1);
      expect(window.location.pathname + window.location.search).toBe(ROUTE);
      const published = context
        .store()
        .getState()
        .settingsAgents.items[0].profiles.find(
          (profile) => profile.id === ACCEPTED,
        ) as DraftProfile;
      expect(published).toMatchObject({
        name: "Live accepted name",
        model: LIVE_MODEL,
        mcp_config: { dirty: true },
      });
      context.state.failMcp = false;
      let retry!: SaveResult;
      await act(async () => {
        retry = await context.coordinator().saveAll();
      });
      expect(retry.failedIds.size).toBe(0);
      const patches = context.requests.filter(
        (request) =>
          request.path === `/api/v1/agent-profiles/${ACCEPTED}` && request.method === "PATCH",
      );
      if (edited) {
        expect(patches).toHaveLength(1);
        expect(JSON.parse(String(patches[0].body))).toMatchObject({
          name: "Genuine draft edit",
          model: LIVE_MODEL,
        });
      } else {
        expect(patches).toHaveLength(0);
      }
      expect(
        context
          .store()
          .getState()
          .settingsAgents.items[0].profiles.find((profile) => profile.id === ACCEPTED),
      ).toMatchObject({
        name: edited ? "Genuine draft edit" : "Live accepted name",
        model: LIVE_MODEL,
      });
      expect(
        context.requests.filter(
          (request) =>
            request.path === `/api/v1/agents/${OWNER}/profiles` && request.method === "POST",
        ),
      ).toHaveLength(1);
      await waitFor(() => expect(window.location.pathname).toBe("/settings/agents/claude-code"));
      expect(context.coordinator().hasDirty).toBe(false);
    },
  );
});
