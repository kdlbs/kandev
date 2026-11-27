import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { deleteAgentProfileAction, updateAgentProfileAction } from "@/app/actions/agents";
import { ApiError } from "@/lib/api/client";
import type { Agent } from "@/lib/types/http";
import { createAppStore } from "@/lib/state/store";
import { useAppStoreApi } from "@/components/state-provider";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import {
  reconcileAgentProfileOptions,
  shouldSyncProfileSaveResponse,
  useProfileDelete,
  useProfileSave,
} from "./agent-profile-page-state";

vi.mock("@/app/actions/agents", () => ({
  deleteAgentProfileAction: vi.fn(),
  updateAgentProfileAction: vi.fn(),
}));
vi.mock("@/components/state-provider", () => ({ useAppStore: vi.fn(), useAppStoreApi: vi.fn() }));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

/** Builds an Agent fixture with the given profile names. */
function agent(id: string, ...profileNames: string[]): Agent {
  return {
    id,
    name: id,
    profiles: profileNames.map((name) => ({
      id: name,
      agentId: id,
      name,
      agentDisplayName: "Test Agent",
      model: "",
      allowIndexing: false,
      autoApprove: false,
      cliFlags: [],
      cliPassthrough: false,
      enabled: true,
      createdAt: "",
      updatedAt: "",
    })),
  } as unknown as Agent;
}

/** Builds an AgentProfileOption fixture with an optional updatedAt. */
function option(id: string, updatedAt = ""): AgentProfileOption {
  return {
    id,
    label: id,
    agent_id: "a1",
    agent_name: "a1",
    cli_passthrough: false,
    updatedAt,
  };
}

describe("reconcileAgentProfileOptions", () => {
  it("rebuilds options from the next agent list", () => {
    const next = reconcileAgentProfileOptions([], [agent("a1", "p1", "p2")]);
    expect(next.map((o) => o.id).sort()).toEqual(["p1", "p2"]);
  });

  it("preserves WS-delivered orphan options the rebuild does not represent", () => {
    // p-orphan was delivered by WS for an agent absent from the next list.
    const next = reconcileAgentProfileOptions([option("p-orphan")], [agent("a1", "p1")]);
    const ids = next.map((o) => o.id);
    expect(ids).toContain("p-orphan");
    expect(ids).toContain("p1");
  });

  it("replaces stale options with the rebuilt versions by id", () => {
    const next = reconcileAgentProfileOptions([option("p1")], [agent("a1", "p1")]);
    expect(next.filter((o) => o.id === "p1")).toHaveLength(1);
    expect(next.find((o) => o.id === "p1")?.label).toContain("p1");
  });

  it("keeps a newer existing option over a stale rebuilt one", () => {
    // WS updated the option (enabled=false, newer revision) while the next
    // agent list carries a stale profile. The rebuild must not regress it.
    const staleProfile = {
      id: "a1",
      name: "a1",
      profiles: [
        {
          id: "p1",
          agentId: "a1",
          name: "p1",
          agentDisplayName: "Test Agent",
          model: "",
          allowIndexing: false,
          autoApprove: false,
          cliFlags: [],
          cliPassthrough: false,
          enabled: true,
          createdAt: "",
          updatedAt: "2026-08-11T21:00:00Z",
        },
      ],
    } as unknown as Agent;
    const newerOption = { ...option("p1", "2026-08-11T22:00:00Z"), enabled: false };

    const next = reconcileAgentProfileOptions([newerOption], [staleProfile]);

    const options = next.filter((o) => o.id === "p1");
    expect(options).toHaveLength(1);
    expect(options[0].enabled).toBe(false);
    expect(options[0].updatedAt).toBe("2026-08-11T22:00:00Z");
  });

  it("keeps a fractional-second newer option over a whole-second rebuilt one", () => {
    // Lexically "…22:00:00Z" sorts after "…22:00:00.100Z" despite being
    // older, so revision precedence must compare instants, not strings.
    const staleProfile = {
      id: "a1",
      name: "a1",
      profiles: [
        {
          id: "p1",
          agentId: "a1",
          name: "p1",
          agentDisplayName: "Test Agent",
          model: "",
          allowIndexing: false,
          autoApprove: false,
          cliFlags: [],
          cliPassthrough: false,
          enabled: true,
          createdAt: "",
          updatedAt: "2026-08-11T22:00:00Z",
        },
      ],
    } as unknown as Agent;
    const fractionalOption = { ...option("p1", "2026-08-11T22:00:00.100Z"), enabled: false };

    const next = reconcileAgentProfileOptions([fractionalOption], [staleProfile]);

    const options = next.filter((o) => o.id === "p1");
    expect(options).toHaveLength(1);
    expect(options[0].enabled).toBe(false);
    expect(options[0].updatedAt).toBe("2026-08-11T22:00:00.100Z");
  });
});

describe("shouldSyncProfileSaveResponse", () => {
  it("rejects a response that is older than a newer websocket baseline", () => {
    const current = agent("a1", "p1").profiles[0];
    const response = { ...current, name: "stale response", updatedAt: "2026-01-01T00:00:00Z" };
    const baseline = { ...current, name: "websocket update", updatedAt: "2026-01-01T01:00:00Z" };

    expect(shouldSyncProfileSaveResponse(response, baseline)).toBe(false);
  });
});

describe("useProfileDelete dynamic profile conflicts", () => {
  const conflictResult = {
    status: "conflict" as const,
    activeSessions: [],
    watchers: [],
    routingTiers: [],
    automations: [],
    utilityAgents: [],
    dynamicProfiles: [{ id: "d1", name: "Balanced selector" }],
  };

  it("keeps dynamic profiles in the conflict and re-requests with force", async () => {
    vi.mocked(deleteAgentProfileAction).mockResolvedValue(conflictResult);
    const owner = agent("a1", "p1");
    const { result } = renderHook(() =>
      useProfileDelete(owner, owner.profiles[0], [owner], vi.fn(), vi.fn()),
    );

    await act(async () => {
      await result.current.handleDeleteProfile();
    });
    expect(result.current.conflict?.dynamicProfiles).toEqual([
      { id: "d1", name: "Balanced selector" },
    ]);

    await act(async () => {
      await result.current.handleForceDelete();
    });
    expect(deleteAgentProfileAction).toHaveBeenLastCalledWith("p1", true);
  });
});

describe("useProfileSave conflict handling", () => {
  it("opens the conflict dialog without also showing the generic toast", async () => {
    const savedProfile = agent("a1", "p1").profiles[0];
    const draft = { ...savedProfile, name: "renamed profile" };
    vi.mocked(updateAgentProfileAction).mockRejectedValue(
      new ApiError("agent profile is used by 1 utility agent(s)", 409, {
        utility_agents: [{ id: "u1", name: "Reviewer" }],
      }),
    );
    const agents = [{ ...agent("a1", "p1"), profiles: [savedProfile] }];
    vi.mocked(useAppStoreApi).mockReturnValue(
      createAppStore({ settingsAgents: { items: agents } }),
    );
    const toast = vi.fn();
    const onProfileConflict = vi.fn();

    const { result } = renderHook(() =>
      useProfileSave({
        agent: agents[0],
        draft,
        savedProfile,
        setSaveStatus: vi.fn(),
        markProfileSubmitted: vi.fn(),
        acceptProfileSaveResponse: () => true,
        toast,
        onProfileConflict,
      }),
    );

    await act(async () => {
      await expect(result.current()).rejects.toBeInstanceOf(ApiError);
    });

    expect(onProfileConflict).toHaveBeenCalledWith({
      utilityAgents: [{ id: "u1", name: "Reviewer" }],
      dynamicProfiles: [],
    });
    expect(toast).not.toHaveBeenCalled();
  });
});

describe("useProfileSave Cursor MCP auth preference", () => {
  it("sends the changed preference when saving the profile editor", async () => {
    const savedProfile = { ...agent("a1", "p1").profiles[0], cursorMcpAuthEnabled: true };
    const draft = { ...savedProfile, cursorMcpAuthEnabled: false };
    vi.mocked(updateAgentProfileAction).mockResolvedValue(draft);
    const agents = [{ ...agent("a1", "p1"), profiles: [savedProfile] }];
    vi.mocked(useAppStoreApi).mockReturnValue(
      createAppStore({ settingsAgents: { items: agents } }),
    );

    const { result } = renderHook(() =>
      useProfileSave({
        agent: agents[0],
        draft,
        savedProfile,
        setSaveStatus: vi.fn(),
        markProfileSubmitted: vi.fn(),
        acceptProfileSaveResponse: () => true,
        toast: vi.fn(),
      }),
    );

    await act(async () => result.current());

    expect(updateAgentProfileAction).toHaveBeenCalledWith(
      "p1",
      expect.objectContaining({ cursor_mcp_auth_enabled: false }),
      false,
    );
  });

  it("omits an unchanged preference from the stale profile draft", async () => {
    const savedProfile = {
      ...agent("a1", "p1").profiles[0],
      cursorMcpAuthEnabled: false,
      cursorPluginsMcpEnabled: false,
    };
    const draft = { ...savedProfile, name: "renamed profile" };
    vi.mocked(updateAgentProfileAction).mockResolvedValue(draft);
    const agents = [{ ...agent("a1", "p1"), profiles: [savedProfile] }];
    vi.mocked(useAppStoreApi).mockReturnValue(
      createAppStore({ settingsAgents: { items: agents } }),
    );

    const { result } = renderHook(() =>
      useProfileSave({
        agent: agents[0],
        draft,
        savedProfile,
        setSaveStatus: vi.fn(),
        markProfileSubmitted: vi.fn(),
        acceptProfileSaveResponse: () => true,
        toast: vi.fn(),
      }),
    );

    await act(async () => result.current());

    expect(updateAgentProfileAction).toHaveBeenLastCalledWith(
      "p1",
      expect.objectContaining({
        cursor_mcp_auth_enabled: undefined,
        cursor_plugins_mcp_enabled: undefined,
      }),
      false,
    );
  });
});
