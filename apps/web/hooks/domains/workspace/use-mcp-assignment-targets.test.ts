import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listMCPServers, listMCPProfileOwners } from "@/lib/api/domains/mcp-api";
import { listAgents } from "@/lib/api/domains/settings-api";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import type { Repository } from "@/lib/types/http";
import type { MCPServerDefinition } from "@/lib/types/http-mcp";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import { useMCPAssignmentTargets } from "./use-mcp-assignment-targets";

vi.mock("@/lib/api/domains/workspace-api", () => ({ listRepositories: vi.fn() }));
vi.mock("@/lib/api/domains/mcp-api", () => ({
  listMCPServers: vi.fn(),
  listMCPProfileOwners: vi.fn(),
}));
vi.mock("@/lib/api/domains/settings-api", () => ({ listAgents: vi.fn() }));
vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessions: vi.fn() }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ fetchTask: vi.fn() }));
const repo: Repository = {
  id: repositoryId("repo"),
  workspace_id: workspaceId("ws"),
  name: "Repository",
  source_type: "local",
  local_path: "/repo",
  provider: "",
  provider_repo_id: "",
  provider_owner: "",
  provider_name: "",
  default_branch: "main",
  worktree_branch_prefix: "",
  pull_before_worktree: false,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: "now",
  updated_at: "now",
};
const definition: MCPServerDefinition = {
  id: "docs",
  workspace_id: "ws",
  display_name: "Docs",
  runtime_name: "docs",
  normalized_runtime_name: "docs",
  enabled: true,
  execution_mode: "remote",
  transport: "sse",
  configuration: { url: "https://example.test" },
  source: "custom",
  revision: 1,
  created_at: "now",
  updated_at: "now",
};
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listRepositories).mockResolvedValue({ repositories: [repo], total: 1 });
  vi.mocked(listMCPServers).mockResolvedValue([definition]);
  vi.mocked(listAgents).mockResolvedValue({ agents: [], total: 0 });
  vi.mocked(listMCPProfileOwners).mockResolvedValue({ workspace_id: "ws", items: [] });
});

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.4, AC-AGENTS-MCP-ASSIGNMENT-003.4
it("reads only the chosen owner type and does no discovery before choosing it", async () => {
  const { result, rerender } = renderHook(
    ({ active }) => useMCPAssignmentTargets("ws", active ? "repository" : null),
    { initialProps: { active: false } },
  );
  expect(listRepositories).not.toHaveBeenCalled();
  rerender({ active: true });
  await waitFor(() => expect(result.current.targets).toHaveLength(1));
  expect(listMCPServers).not.toHaveBeenCalled();
  expect(listAgents).not.toHaveBeenCalled();
  expect(listTaskSessions).not.toHaveBeenCalled();
  expect(fetchTask).not.toHaveBeenCalled();
});

it("waits for an explicit task before reading sessions", () => {
  const { result } = renderHook(() => useMCPAssignmentTargets("ws", "task_session"));
  expect(result.current.loading).toBe(false);
  expect(listTaskSessions).not.toHaveBeenCalled();
  expect(fetchTask).not.toHaveBeenCalled();
});

it("discards late owner lists after leaving and returning to the same workspace", async () => {
  const old = deferred<Awaited<ReturnType<typeof listRepositories>>>();
  vi.mocked(listRepositories).mockReturnValueOnce(old.promise);
  const { result, rerender } = renderHook(({ w }) => useMCPAssignmentTargets(w, "repository"), {
    initialProps: { w: "ws" },
  });
  rerender({ w: "other" });
  rerender({ w: "ws" });
  await waitFor(() => expect(result.current.targets[0]?.ownerId).toBe("repo"));
  await act(async () =>
    old.resolve({ repositories: [{ ...repo, id: repositoryId("old") }], total: 1 }),
  );
  expect(result.current.targets.map((target) => target.ownerId)).toEqual(["repo"]);
});

it("revalidates the saved definition and owner before constructing a focus-only destination", async () => {
  const { result } = renderHook(() => useMCPAssignmentTargets("ws", "repository"));
  await waitFor(() => expect(result.current.targets).toHaveLength(1));
  await act(async () => {
    expect(await result.current.openHref(result.current.targets[0], "docs")).toBe(
      "/settings/workspaces/ws/repositories?repositoryId=repo&mcpServerId=docs#mcp",
    );
  });
  expect(listRepositories).toHaveBeenCalledTimes(2);
  expect(listMCPServers).toHaveBeenCalledTimes(1);
});

it.each(["removed-owner", "disabled-definition", "read-failed"])(
  "retains choices after %s and allows a read-only recovery",
  async (failure) => {
    const { result } = renderHook(() => useMCPAssignmentTargets("ws", "repository"));
    await waitFor(() => expect(result.current.targets).toHaveLength(1));
    if (failure === "removed-owner")
      vi.mocked(listRepositories).mockResolvedValueOnce({ repositories: [], total: 0 });
    if (failure === "disabled-definition")
      vi.mocked(listMCPServers).mockResolvedValueOnce([{ ...definition, enabled: false }]);
    if (failure === "read-failed")
      vi.mocked(listMCPServers).mockRejectedValueOnce(new Error("private details"));
    await act(async () => {
      expect(await result.current.openHref(result.current.targets[0], "docs")).toBeNull();
    });
    expect(result.current.openStatus).toBe(failure === "read-failed" ? "failed" : "unavailable");
    expect(result.current.targets).toHaveLength(1);
    await act(async () => {
      expect(await result.current.openHref(result.current.targets[0], "docs")).toContain(
        "mcpServerId=docs",
      );
    });
    expect(result.current.openStatus).toBe("idle");
  },
);

it("blocks same-tick duplicate open and ignores verification after the chooser context closes", async () => {
  const pending = deferred<MCPServerDefinition[]>();
  vi.mocked(listMCPServers).mockReturnValueOnce(pending.promise);
  const { result, rerender } = renderHook(
    ({ active }) => useMCPAssignmentTargets("ws", active ? "repository" : null),
    { initialProps: { active: true } },
  );
  await waitFor(() => expect(result.current.targets).toHaveLength(1));
  let first!: Promise<string | null>;
  let second!: Promise<string | null>;
  act(() => {
    const target = result.current.targets[0];
    first = result.current.openHref(target, "docs");
    second = result.current.openHref(target, "docs");
  });
  expect(listMCPServers).toHaveBeenCalledTimes(1);
  rerender({ active: false });
  await act(async () => {
    pending.resolve([definition]);
    expect(await first).toBeNull();
    expect(await second).toBeNull();
  });
  expect(result.current.openStatus).toBe("idle");
  expect(result.current.targets).toEqual([]);
});

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.2, AC-AGENTS-MCP-ASSIGNMENT-003.3
it("reads authorized metadata across cursor pages, including workspace profiles absent from global agents", async () => {
  vi.mocked(listMCPProfileOwners)
    .mockResolvedValueOnce({ workspace_id: "ws", items: [], next_cursor: "after" })
    .mockResolvedValueOnce({
      workspace_id: "ws",
      items: [
        {
          id: "workspace-profile",
          agent_id: "agent",
          agent_name: "Agent",
          name: "Workspace profile",
          workspace_id: "ws",
          enabled: true,
          kind: "concrete",
          supports_mcp: true,
        },
      ],
    });
  const { result } = renderHook(() => useMCPAssignmentTargets("ws", "profile"));
  await waitFor(() =>
    expect(result.current.targets.map((item) => item.ownerId)).toEqual(["workspace-profile"]),
  );
  expect(listAgents).not.toHaveBeenCalled();
  expect(listMCPProfileOwners).toHaveBeenNthCalledWith(
    2,
    "ws",
    "after",
    expect.objectContaining({ init: { signal: expect.any(AbortSignal) } }),
  );
});

it("fails closed on a repeated metadata cursor instead of looping or exposing partial inventory", async () => {
  vi.mocked(listMCPProfileOwners).mockResolvedValue({
    workspace_id: "ws",
    items: [],
    next_cursor: "repeat",
  });
  const { result } = renderHook(() => useMCPAssignmentTargets("ws", "profile"));
  await waitFor(() => expect(result.current.failed).toBe(true));
  expect(listMCPProfileOwners).toHaveBeenCalledTimes(2);
  expect(result.current.targets).toEqual([]);
});
