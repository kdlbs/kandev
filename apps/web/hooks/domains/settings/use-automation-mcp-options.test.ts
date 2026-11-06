import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  useAutomationMCPOptions,
  useAutomationMCPSelectionReload,
} from "./use-automation-mcp-options";
const api = vi.hoisted(() => ({ options: vi.fn(), get: vi.fn() }));
vi.mock("@/lib/api/domains/automation-api", () => ({
  getAutomationMCPOptions: api.options,
  getAutomation: api.get,
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
const options = (id: string) => ({
  packages: [],
  definitions: [{ id, name: id, enabled: true }],
  inherited: [],
  task_defaults: ["core"],
  task_blocked: [],
});
describe("automation MCP options", () => {
  it("settles a failed refresh while retaining this workspace's previous options", async () => {
    api.options.mockResolvedValueOnce(options("saved")).mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(() => useAutomationMCPOptions("workspace", "profile", []));
    await waitFor(() => expect(result.current.options).toEqual(options("saved")));
    await act(async () => {
      await result.current.reload();
    });
    expect(result.current.error).toBe(true);
    expect(result.current.loading).toBe(false);
    expect(result.current.options).toEqual(options("saved"));
  });
  it("ignores stale workspace results", async () => {
    let finish!: (value: ReturnType<typeof options>) => void;
    api.options
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            finish = done;
          }),
      )
      .mockResolvedValueOnce(options("new"));
    const { result, rerender } = renderHook(
      ({ workspace }) => useAutomationMCPOptions(workspace, "profile", []),
      { initialProps: { workspace: "old" } },
    );
    rerender({ workspace: "new" });
    await waitFor(() => expect(result.current.options).toEqual(options("new")));
    await act(async () => finish(options("old")));
    expect(result.current.options).toEqual(options("new"));
    expect(result.current.loading).toBe(false);
  });
  it("loads a new context when profile or repository changes", async () => {
    api.options.mockResolvedValue(options("server"));
    const { rerender } = renderHook(
      ({ profile, repos }) => useAutomationMCPOptions("workspace", profile, repos),
      { initialProps: { profile: "first", repos: [] as string[] } },
    );
    await waitFor(() => expect(api.options).toHaveBeenCalledWith("workspace", "first", []));
    rerender({ profile: "second", repos: ["repo"] });
    await waitFor(() => expect(api.options).toHaveBeenCalledWith("workspace", "second", ["repo"]));
  });
  it("ignores an options callback retained from another workspace", async () => {
    api.options.mockResolvedValueOnce(options("old")).mockResolvedValueOnce(options("new"));
    const { result, rerender } = renderHook(
      ({ workspace }) => useAutomationMCPOptions(workspace, "profile", []),
      { initialProps: { workspace: "old" } },
    );
    await waitFor(() => expect(result.current.options).toEqual(options("old")));
    const staleReload = result.current.reload;
    rerender({ workspace: "new" });
    await waitFor(() => expect(result.current.options).toEqual(options("new")));
    await act(async () => {
      await staleReload();
    });
    expect(api.options).toHaveBeenCalledTimes(2);
    expect(result.current.options).toEqual(options("new"));
  });
});

describe("saved automation MCP selection", () => {
  it("does not reconcile a late saved selection into a different editor", async () => {
    let finish!: (value: unknown) => void;
    api.get.mockImplementationOnce(
      () =>
        new Promise((done) => {
          finish = done;
        }),
    );
    const { result, rerender } = renderHook(
      ({ workspace, id }) => useAutomationMCPSelectionReload(workspace, id),
      { initialProps: { workspace: "old", id: "automation" } },
    );
    const pending = result.current();
    rerender({ workspace: "new", id: "other" });
    await act(async () =>
      finish({ workspace_id: "old", id: "automation", mcp_selection_revision: 4 }),
    );
    expect(await pending).toBeUndefined();
  });
  it("rejects a stale reload callback invoked after the editor changes", async () => {
    api.get.mockResolvedValue({ id: "old", workspace_id: "old" });
    const { result, rerender } = renderHook(
      ({ workspace, id }) => useAutomationMCPSelectionReload(workspace, id),
      { initialProps: { workspace: "old", id: "old" } },
    );
    const staleReload = result.current;
    rerender({ workspace: "new", id: "new" });
    expect(await staleReload()).toBeUndefined();
    expect(api.get).not.toHaveBeenCalled();
  });
  it("clears a failed unsaved editor after reloading choices", async () => {
    const { useAutomationMCPReconciliation } = await import("./use-automation-mcp-reconciliation");
    const { result } = renderHook(() =>
      useAutomationMCPReconciliation("workspace", null, vi.fn(), vi.fn()),
    );
    act(() => result.current.failed());
    expect(result.current.saveFailed).toBe(true);
    await act(() => result.current.reload());
    expect(result.current.saveFailed).toBe(false);
  });
  it("rejects an older overlapping saved-selection reload", async () => {
    let finish!: (value: unknown) => void;
    api.get
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            finish = done;
          }),
      )
      .mockResolvedValueOnce({
        id: "automation",
        workspace_id: "workspace",
        mcp_selection_revision: 4,
      });
    const { result } = renderHook(() => useAutomationMCPSelectionReload("workspace", "automation"));
    const older = result.current();
    expect((await result.current())?.mcp_selection_revision).toBe(4);
    finish({ id: "automation", workspace_id: "workspace", mcp_selection_revision: 3 });
    expect(await older).toBeUndefined();
  });
  it("retains the recovery action when saved selection reload fails", async () => {
    const { useAutomationMCPReconciliation } = await import("./use-automation-mcp-reconciliation");
    api.get.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() =>
      useAutomationMCPReconciliation("workspace", "automation", vi.fn(), vi.fn()),
    );
    act(() => result.current.failed());
    await act(async () => {
      await result.current.reload();
    });
    expect(result.current.saveFailed).toBe(true);
  });
});
