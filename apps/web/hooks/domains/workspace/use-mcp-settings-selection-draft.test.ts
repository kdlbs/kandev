import { createElement, Suspense, startTransition, useLayoutEffect } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getMCPSelections, replaceMCPSelections } from "@/lib/api/domains/mcp-api";
import type { MCPSelectionResponse } from "@/lib/types/http-mcp";
import { useMCPSettingsSelectionDraft } from "./use-mcp-settings-selection-draft";
vi.mock("@/lib/api/domains/mcp-api", () => ({
  getMCPSelections: vi.fn(),
  replaceMCPSelections: vi.fn(),
}));
const selection = (workspace = "a", ids = ["saved"]): MCPSelectionResponse => ({
  scope: "profile",
  owner_id: "profile",
  workspace_id: workspace,
  definition_ids: ids,
});
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getMCPSelections).mockImplementation(async (_s, _o, w) => selection(w));
});
afterEach(cleanup);

// @covers AC-AGENTS-MCP-MANAGEMENT-002.2, AC-AGENTS-MCP-MANAGEMENT-002.3
describe("settings-owned MCP draft", () => {
  it("accepts a canonical save baseline and preserves edits made while saving", async () => {
    let resolve!: (response: MCPSelectionResponse) => void;
    vi.mocked(replaceMCPSelections).mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    const { result } = renderHook(() => useMCPSettingsSelectionDraft("profile", "profile", "a"));
    await waitFor(() => expect(result.current.selectedIds).toEqual(["saved"]));
    act(() => result.current.setSelectedIds(["submitted"]));
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.save();
    });
    act(() => result.current.setSelectedIds(["newer"]));
    await act(async () => {
      resolve(selection("a", ["canonical"]));
      await pending;
    });
    expect(result.current.selectedIds).toEqual(["newer"]);
    expect(result.current.dirty).toBe(true);
    act(() => result.current.discard());
    expect(result.current.selectedIds).toEqual(["canonical"]);
    expect(result.current.dirty).toBe(false);
  });
  it("retains a rejected draft and uses canonical data when there are no newer edits", async () => {
    const { result } = renderHook(() => useMCPSettingsSelectionDraft("profile", "profile", "a"));
    await waitFor(() => expect(result.current.selectedIds).toEqual(["saved"]));
    act(() => result.current.setSelectedIds(["draft"]));
    vi.mocked(replaceMCPSelections)
      .mockRejectedValueOnce(new Error("save failed"))
      .mockResolvedValueOnce(selection("a", ["canonical"]));
    await act(async () => {
      await expect(result.current.save()).rejects.toThrow("save failed");
    });
    expect(result.current.dirty).toBe(true);
    expect(result.current.selectedIds).toEqual(["draft"]);
    await act(async () => {
      await expect(result.current.save()).rejects.toThrow("MCP selection is not ready to save");
    });
    expect(replaceMCPSelections).toHaveBeenCalledTimes(1);
    await act(() => result.current.editor.reload());
    expect(result.current.selectedIds).toEqual(["draft"]);
    await act(() => result.current.save());
    expect(result.current.selectedIds).toEqual(["canonical"]);
    expect(result.current.dirty).toBe(false);
  });
  it("does not promote an obsolete save or show the previous workspace draft", async () => {
    let resolve!: (response: MCPSelectionResponse) => void;
    vi.mocked(replaceMCPSelections).mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    const { result, rerender } = renderHook(
      ({ w }) => useMCPSettingsSelectionDraft("profile", "profile", w),
      { initialProps: { w: "a" } },
    );
    await waitFor(() => expect(result.current.selectedIds).toEqual(["saved"]));
    act(() => result.current.setSelectedIds(["draft"]));
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.save();
    });
    rerender({ w: "b" });
    expect(result.current.selectedIds).not.toContain("draft");
    await waitFor(() => expect(result.current.editor.selection?.workspace_id).toBe("b"));
    await act(async () => {
      resolve(selection("a", ["old"]));
      expect(await pending).toBeNull();
    });
    expect(result.current.selectedIds).toEqual(["saved"]);
    expect(result.current.dirty).toBe(false);
  });
});

it("submits only the committed draft while a newer edit is suspended", async () => {
  let current!: ReturnType<typeof useMCPSettingsSelectionDraft>;
  let suspended = false;
  const never = new Promise<void>(() => {});
  vi.mocked(replaceMCPSelections).mockResolvedValue(selection("a", ["committed"]));
  function Consumer() {
    const draft = useMCPSettingsSelectionDraft("profile", "profile", "a");
    useLayoutEffect(() => {
      current = draft;
    });
    if (draft.selectedIds.includes("speculative")) {
      suspended = true;
      throw never;
    }
    return createElement("span", null, draft.selectedIds.join(","));
  }
  const view = render(createElement(Suspense, { fallback: null }, createElement(Consumer)));
  await waitFor(() => expect(current.selectedIds).toEqual(["saved"]));
  act(() => current.setSelectedIds(["committed"]));
  act(() => startTransition(() => current.setSelectedIds(["speculative"])));
  expect(suspended).toBe(true);
  expect(view.container.textContent).toBe("committed");
  await act(() => current.save());
  expect(replaceMCPSelections).toHaveBeenCalledWith("profile", "profile", "a", ["committed"], {
    expected_definition_ids: ["saved"],
  });
});
