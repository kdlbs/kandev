import { createElement, Suspense, startTransition, useLayoutEffect, useState } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { getMCPSelections, replaceMCPSelections } from "@/lib/api/domains/mcp-api";
import { useMCPSelectionDraft } from "./use-mcp-selection-draft";
import { useMCPSelectionEditor } from "./use-mcp-selection-editor";

vi.mock("@/lib/api/domains/mcp-api", () => ({
  getMCPSelections: vi.fn(),
  replaceMCPSelections: vi.fn(),
}));
afterEach(cleanup);
const selection = (owner_id: string, workspace_id = "ws") => ({
  scope: "task_session" as const,
  owner_id,
  workspace_id,
  definition_ids: [] as string[],
});

it.each(["resolve", "reject"])(
  "an old save's %s cannot invalidate its replacement context's initial load",
  async (outcome) => {
    vi.resetAllMocks();
    let finishSave!: (value: ReturnType<typeof selection>) => void;
    let rejectSave!: (error: Error) => void;
    let finishLoad!: (value: ReturnType<typeof selection>) => void;
    vi.mocked(getMCPSelections).mockResolvedValueOnce(selection("a"));
    vi.mocked(replaceMCPSelections).mockReturnValue(
      new Promise((resolve, reject) => {
        finishSave = resolve;
        rejectSave = reject;
      }),
    );
    const { result, rerender } = renderHook(
      ({ owner, workspace }) => useMCPSelectionEditor("task_session", owner, workspace),
      { initialProps: { owner: "a", workspace: "ws" } },
    );
    await waitFor(() => expect(result.current.selection?.owner_id).toBe("a"));
    let saving!: Promise<unknown>;
    act(() => {
      saving = result.current.save([]).catch(() => undefined);
    });
    vi.mocked(getMCPSelections).mockReturnValueOnce(
      new Promise((resolve) => {
        finishLoad = resolve;
      }),
    );
    rerender({ owner: "b", workspace: "ws-b" });
    await act(async () => {
      if (outcome === "resolve") finishSave(selection("a"));
      else rejectSave(new Error("old context failure"));
      await saving;
    });
    await act(async () => {
      finishLoad(selection("b", "ws-b"));
    });
    expect(result.current.selection).toEqual(selection("b", "ws-b"));
    expect(result.current.loading).toBe(false);
    expect(result.current.saving).toBe(false);
    expect(result.current.error).toBeNull();
  },
);

it("settles an accepted save and draft after a replacement context render is discarded", async () => {
  vi.resetAllMocks();
  vi.mocked(getMCPSelections).mockResolvedValue(selection("a"));
  let finish!: (value: ReturnType<typeof selection> & { definition_ids: string[] }) => void;
  vi.mocked(replaceMCPSelections).mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  let current!: {
    editor: ReturnType<typeof useMCPSelectionEditor>;
    draft: ReturnType<typeof useMCPSelectionDraft>;
  };
  let changeOwner!: (owner: string) => void;
  let suspended = false;
  const never = new Promise<void>(() => {});
  function Consumer() {
    const [owner, setOwner] = useState("a");
    const editor = useMCPSelectionEditor("task_session", owner, "ws");
    const draft = useMCPSelectionDraft(owner, editor.selection);
    useLayoutEffect(() => {
      current = { editor, draft };
      changeOwner = setOwner;
    });
    if (owner === "b") {
      suspended = true;
      throw never;
    }
    return createElement("span", null, owner);
  }
  render(createElement(Suspense, { fallback: null }, createElement(Consumer)));
  await waitFor(() => expect(current.editor.selection?.owner_id).toBe("a"));
  act(() => current.draft.setSelectedIds(["docs"]));
  let pending!: Promise<unknown>;
  act(() => {
    pending = current.editor.save(["docs"]).then((saved) => {
      current.draft.acknowledge(saved);
      return saved;
    });
  });
  act(() => startTransition(() => changeOwner("b")));
  expect(suspended).toBe(true);
  await act(async () => {
    finish({ ...selection("a"), definition_ids: ["docs"] });
    expect(await pending).not.toBeNull();
  });
  expect(current.editor.saving).toBe(false);
  expect(current.draft.selectedIds).toEqual(["docs"]);
  expect(current.draft.dirty).toBe(false);
});
