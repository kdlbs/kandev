import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { getMCPSelections, replaceMCPSelections } from "@/lib/api/domains/mcp-api";
import { useMCPSelectionEditor } from "./use-mcp-selection-editor";

vi.mock("@/lib/api/domains/mcp-api", () => ({
  getMCPSelections: vi.fn(),
  replaceMCPSelections: vi.fn(),
}));
const selection = (owner_id: string, workspace_id = "ws") => ({
  scope: "task_session" as const,
  owner_id,
  workspace_id,
  definition_ids: [],
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
