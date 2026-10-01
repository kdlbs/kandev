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
