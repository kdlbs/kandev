import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getMCPSelections, replaceMCPSelections } from "@/lib/api/domains/mcp-api";
import { ApiError } from "@/lib/api/client";
import type { MCPSelectionResponse } from "@/lib/types/http-mcp";
import { useMCPSelectionEditor } from "./use-mcp-selection-editor";

vi.mock("@/lib/api/domains/mcp-api", () => ({
  getMCPSelections: vi.fn(),
  replaceMCPSelections: vi.fn(),
}));

const WORKSPACE_A = "workspace-a";

function selection(ownerId = "session-a", workspaceId = WORKSPACE_A): MCPSelectionResponse {
  return {
    scope: "task_session",
    owner_id: ownerId,
    workspace_id: workspaceId,
    definition_ids: ["saved-mcp"],
    mcp_state: { desired_revision: 2, applied_revision: 2, apply_state: "applied" },
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getMCPSelections).mockImplementation(async (_scope, ownerId, workspaceId) =>
    selection(ownerId, workspaceId),
  );
});

describe("MCP selection write authority", () => {
  // @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1
  it("promotes only the successful response to desired selection state", async () => {
    const next = {
      ...selection(),
      definition_ids: ["extra-mcp"],
      mcp_state: { desired_revision: 3, applied_revision: 2, apply_state: "pending_idle" as const },
    };
    vi.mocked(replaceMCPSelections).mockResolvedValue(next);
    const { result } = renderHook(() =>
      useMCPSelectionEditor("task_session", "session-a", WORKSPACE_A),
    );
    await waitFor(() => expect(result.current.selection).toEqual(selection()));
    await act(async () => {
      await result.current.save(["extra-mcp"]);
    });
    expect(result.current.selection).toEqual(next);
    expect(result.current.saving).toBe(false);
  });

  it("retains saved IDs and revisions when the selection write is rejected", async () => {
    const error = new ApiError("forbidden", 403, { error_code: "forbidden" });
    vi.mocked(replaceMCPSelections).mockRejectedValue(error);
    const { result } = renderHook(() =>
      useMCPSelectionEditor("task_session", "session-a", WORKSPACE_A),
    );
    await waitFor(() => expect(result.current.selection).toEqual(selection()));
    await act(async () => {
      await expect(result.current.save(["extra-mcp"])).rejects.toBe(error);
    });
    expect(result.current.selection).toEqual(selection());
    expect(result.current.saveError).toBe(error);
    expect(result.current.saving).toBe(false);
  });

  it.each(["resolve", "reject"])(
    "ignores a late %s after the session and workspace change",
    async (outcome) => {
      let resolve!: (value: MCPSelectionResponse) => void;
      let reject!: (error: Error) => void;
      vi.mocked(replaceMCPSelections).mockReturnValue(
        new Promise((yes, no) => {
          resolve = yes;
          reject = no;
        }),
      );
      const { result, rerender } = renderHook(
        ({ owner, workspace }) => useMCPSelectionEditor("task_session", owner, workspace),
        { initialProps: { owner: "session-a", workspace: WORKSPACE_A } },
      );
      await waitFor(() => expect(result.current.selection).toEqual(selection()));
      let pending!: Promise<unknown>;
      act(() => {
        pending = result.current.save(["extra-mcp"]).catch(() => undefined);
      });
      rerender({ owner: "session-b", workspace: "workspace-b" });
      await waitFor(() =>
        expect(result.current.selection).toEqual(selection("session-b", "workspace-b")),
      );
      await act(async () => {
        if (outcome === "resolve") resolve({ ...selection(), definition_ids: ["extra-mcp"] });
        else reject(new Error("old request rejected"));
        await pending;
      });
      expect(result.current.selection).toEqual(selection("session-b", "workspace-b"));
      expect(result.current.saveError).toBeNull();
      expect(result.current.saving).toBe(false);
    },
  );
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe("MCP selection refresh", () => {
  // @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.11
  it.each([false, true])(
    "retains loaded selection while a background refresh is unresolved (empty %s)",
    async (empty) => {
      const saved = { ...selection(), definition_ids: empty ? [] : ["saved-mcp"] };
      vi.mocked(getMCPSelections).mockResolvedValueOnce(saved);
      const { result } = renderHook(() =>
        useMCPSelectionEditor("task_session", "session-a", WORKSPACE_A),
      );
      await waitFor(() => expect(result.current.selection).toEqual(saved));
      const next = deferred<MCPSelectionResponse>();
      vi.mocked(getMCPSelections).mockReturnValueOnce(next.promise);
      let pending!: Promise<unknown>;
      act(() => {
        pending = result.current.reload();
      });
      expect(result.current.loading).toBe(false);
      expect(result.current.selection).toEqual(saved);
      await act(async () => {
        next.resolve(saved);
        await pending;
      });
      expect(result.current.loading).toBe(false);
    },
  );

  it("retains loaded data after failed refresh and clears error after recovery", async () => {
    const { result } = renderHook(() =>
      useMCPSelectionEditor("task_session", "session-a", WORKSPACE_A),
    );
    await waitFor(() => expect(result.current.selection).toEqual(selection()));
    const error = new Error("refresh rejected");
    vi.mocked(getMCPSelections).mockRejectedValueOnce(error);
    await act(async () => {
      await result.current.reload();
    });
    expect(result.current.selection).toEqual(selection());
    expect(result.current.loadError).toBe(error);
    expect(result.current.loading).toBe(false);
    await act(async () => {
      await result.current.reload();
    });
    expect(result.current.loadError).toBeNull();
  });

  it("masks previous context while replacement selection is loading and ignores its late refresh", async () => {
    const { result, rerender } = renderHook(
      ({ owner }) => useMCPSelectionEditor("task_session", owner, WORKSPACE_A),
      { initialProps: { owner: "session-a" } },
    );
    await waitFor(() => expect(result.current.selection).toEqual(selection()));
    const old = deferred<MCPSelectionResponse>();
    const replacement = deferred<MCPSelectionResponse>();
    vi.mocked(getMCPSelections)
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(replacement.promise);
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.reload();
    });
    rerender({ owner: "session-b" });
    expect(result.current.selection).toBeNull();
    await act(async () => {
      old.resolve(selection());
      await pending;
    });
    expect(result.current.selection).toBeNull();
    await act(async () => {
      replacement.resolve(selection("session-b"));
    });
    expect(result.current.selection).toEqual(selection("session-b"));
  });

  it("a successful save wins over an unresolved refresh without stuck loading", async () => {
    const { result } = renderHook(() =>
      useMCPSelectionEditor("task_session", "session-a", WORKSPACE_A),
    );
    await waitFor(() => expect(result.current.selection).toEqual(selection()));
    const old = deferred<MCPSelectionResponse>();
    vi.mocked(getMCPSelections).mockReturnValueOnce(old.promise);
    const saved = { ...selection(), definition_ids: ["new-mcp"] };
    vi.mocked(replaceMCPSelections).mockResolvedValueOnce(saved);
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.reload();
    });
    await act(async () => {
      await result.current.save(["new-mcp"]);
    });
    await act(async () => {
      old.resolve(selection());
      await pending;
    });
    expect(result.current.selection).toEqual(saved);
    expect(result.current.loading).toBe(false);
    expect(result.current.saving).toBe(false);
  });
});

describe("conditional selection writes", () => {
  // @covers AC-AGENTS-MCP-ASSIGNMENT-003.1
  it.each(["repository", "task_session"] as const)(
    "sends the accepted %s snapshot",
    async (scope) => {
      const saved = { ...selection(), scope, selection_revision: 2 };
      vi.mocked(getMCPSelections).mockResolvedValue(saved);
      vi.mocked(replaceMCPSelections).mockResolvedValue({ ...saved, definition_ids: ["new"] });
      const { result } = renderHook(() => useMCPSelectionEditor(scope, "session-a", WORKSPACE_A));
      await waitFor(() => expect(result.current.selection).toEqual(saved));
      await act(async () => {
        await result.current.save(["new"]);
      });
      expect(replaceMCPSelections).toHaveBeenCalledWith(
        scope,
        "session-a",
        WORKSPACE_A,
        ["new"],
        scope === "task_session"
          ? { expected_revision: 2 }
          : { expected_definition_ids: ["saved-mcp"] },
      );
    },
  );

  it("blocks a second same-tick write until the first settles", async () => {
    const pending = deferred<MCPSelectionResponse>();
    vi.mocked(replaceMCPSelections).mockReturnValue(pending.promise);
    const { result } = renderHook(() =>
      useMCPSelectionEditor("task_session", "session-a", WORKSPACE_A),
    );
    await waitFor(() => expect(result.current.selection).not.toBeNull());
    let first!: Promise<unknown>;
    await act(async () => {
      first = result.current.save(["first"]);
      await expect(result.current.save(["second"])).rejects.toBeDefined();
    });
    expect(replaceMCPSelections).toHaveBeenCalledTimes(1);
    await act(async () => {
      pending.resolve(selection());
      await first;
    });
  });
});
