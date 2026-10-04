import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentProjectContext } from "./use-agent-project-context";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  read: vi.fn(),
  write: vi.fn(),
}));

vi.mock("@/lib/api/domains/agent-projects-api", () => ({
  listAgentProjectContext: mocks.list,
  readAgentProjectContextFile: mocks.read,
  writeAgentProjectContextFile: mocks.write,
}));

beforeEach(() => {
  vi.resetAllMocks();
  mocks.list.mockResolvedValue({ entries: [] });
});

describe("useAgentProjectContext project switches", () => {
  it("ignores an old project's save response after switching projects", async () => {
    let finishSave!: (value: { hash: string }) => void;
    mocks.write.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishSave = resolve;
        }),
    );
    mocks.read.mockResolvedValue({ path: "notes.md", content: "original", hash: "hash-a" });
    const { result, rerender } = renderHook(
      ({ projectId }) => useAgentProjectContext("workspace", projectId),
      { initialProps: { projectId: "project-a" } },
    );
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => result.current.openFile("notes.md"));
    act(() => result.current.setDraft("saved for project A"));
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.save();
    });
    rerender({ projectId: "project-b" });
    await waitFor(() => expect(mocks.list).toHaveBeenLastCalledWith("workspace", "project-b", ""));
    await act(async () => {
      finishSave({ hash: "hash-saved-a" });
      await pending;
    });
    expect(result.current.file).toBeNull();
    expect(result.current.busy).toBe(false);
  });

  it("clears a pending file read when switching projects", async () => {
    let finishRead!: (value: { path: string; content: string; hash: string }) => void;
    mocks.read.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishRead = resolve;
        }),
    );
    const { result, rerender } = renderHook(
      ({ projectId }) => useAgentProjectContext("workspace", projectId),
      { initialProps: { projectId: "project-a" } },
    );
    await waitFor(() => expect(result.current.status).toBe("ready"));
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.openFile("notes.md");
    });
    rerender({ projectId: "project-b" });
    await waitFor(() => expect(mocks.list).toHaveBeenLastCalledWith("workspace", "project-b", ""));
    await act(async () => {
      finishRead({ path: "notes.md", content: "project A", hash: "hash-a" });
      await pending;
    });
    expect(result.current.file).toBeNull();
    expect(result.current.busy).toBe(false);
  });
});

describe("useAgentProjectContext save failures", () => {
  it("keeps a draft after a conflicting context write", async () => {
    mocks.read.mockResolvedValue({ path: "notes.md", content: "original", hash: "hash-a" });
    mocks.write.mockRejectedValue(new Error("context changed"));
    const { result } = renderHook(() => useAgentProjectContext("workspace", "project"));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => result.current.openFile("notes.md"));
    act(() => result.current.setDraft("keep this draft"));
    await act(async () => result.current.save());
    expect(result.current.draft).toBe("keep this draft");
    expect(result.current.error).toBe("context changed");
    expect(result.current.dirty).toBe(true);
    expect(result.current.busy).toBe(false);
  });

  it("ignores an old save failure while the new project's file is loading", async () => {
    let failSave!: (error: Error) => void;
    let finishRead!: (value: { path: string; content: string; hash: string }) => void;
    mocks.write.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          failSave = reject;
        }),
    );
    mocks.read.mockResolvedValueOnce({ path: "notes.md", content: "project A", hash: "a" });
    const { result, rerender } = renderHook(
      ({ projectId }) => useAgentProjectContext("workspace", projectId),
      { initialProps: { projectId: "project-a" } },
    );
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => result.current.openFile("notes.md"));
    act(() => result.current.setDraft("edited A"));
    let pendingSave!: Promise<void>;
    act(() => {
      pendingSave = result.current.save();
    });
    rerender({ projectId: "project-b" });
    await waitFor(() => expect(mocks.list).toHaveBeenLastCalledWith("workspace", "project-b", ""));
    mocks.read.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishRead = resolve;
        }),
    );
    let pendingRead!: Promise<void>;
    act(() => {
      pendingRead = result.current.openFile("notes.md");
    });
    await act(async () => {
      failSave(new Error("old save failed"));
      await pendingSave;
    });
    expect(result.current.error).toBeNull();
    expect(result.current.busy).toBe(true);
    await act(async () => {
      finishRead({ path: "notes.md", content: "project B", hash: "b" });
      await pendingRead;
    });
    expect(result.current.file?.content).toBe("project B");
    expect(result.current.busy).toBe(false);
  });
});

describe("useAgentProjectContext navigation", () => {
  it("returns to the containing directory after opening a nested file", async () => {
    const { result } = renderHook(() => useAgentProjectContext("workspace", "project"));
    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith("workspace", "project", ""));

    await act(async () => result.current.loadDirectory("docs"));
    await act(async () => {
      mocks.read.mockResolvedValue({ path: "docs/guide.md", content: "guide", hash: "hash" });
      await result.current.openFile("docs/guide.md");
    });
    await act(async () => result.current.goUp());

    expect(mocks.list).toHaveBeenLastCalledWith("workspace", "project", "docs");
    expect(result.current.directory).toBe("docs");
    expect(result.current.file).toBeNull();
  });

  it("ascends one level when viewing a directory", async () => {
    const { result } = renderHook(() => useAgentProjectContext("workspace", "project"));
    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith("workspace", "project", ""));

    await act(async () => result.current.loadDirectory("docs/guides"));
    await act(async () => result.current.goUp());

    expect(mocks.list).toHaveBeenLastCalledWith("workspace", "project", "docs");
    expect(result.current.directory).toBe("docs");
  });
});
