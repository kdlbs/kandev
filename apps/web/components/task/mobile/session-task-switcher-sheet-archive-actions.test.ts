import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSheetArchiveActions } from "./session-task-switcher-sheet-hooks";

describe("useSheetArchiveActions", () => {
  it("tracks the directly archived row while its request is pending", async () => {
    let resolveArchive: () => void = () => undefined;
    const archiveAndSwitch = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveArchive = resolve;
        }),
    );
    const store = { getState: () => ({}) } as Parameters<typeof useSheetArchiveActions>[0];
    const { result } = renderHook(() =>
      useSheetArchiveActions(
        store,
        archiveAndSwitch as Parameters<typeof useSheetArchiveActions>[1],
      ),
    );

    act(() => result.current.handleArchiveTask("task-1", { cascade: false }));

    await waitFor(() => expect(result.current.archivingTaskId).toBe("task-1"));
    expect(result.current.isArchiving).toBe(true);
    await act(async () => resolveArchive());
    await waitFor(() => expect(result.current.archivingTaskId).toBeNull());
    expect(result.current.isArchiving).toBe(false);
  });
});
