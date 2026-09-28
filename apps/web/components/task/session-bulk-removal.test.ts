import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import {
  buildBulkSessionRemovalSnapshot,
  executeBulkSessionRemoval,
  isBulkSessionRemovalSnapshotCurrent,
  useBulkSessionRemoval,
} from "./session-bulk-removal";

const taskSession = (id: string, state: TaskSession["state"], isPrimary = false): TaskSession =>
  ({
    id,
    task_id: "task-a",
    state,
    is_primary: isPrimary,
    started_at: "2026-09-27T00:00:00Z",
  }) as TaskSession;

describe("bulk session removal", () => {
  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.2 targets hidden task sessions, not panels", () => {
    const sessions = [
      taskSession("primary", "COMPLETED", true),
      taskSession("selected", "WAITING_FOR_INPUT"),
      taskSession("hidden", "COMPLETED"),
    ];

    expect(buildBulkSessionRemovalSnapshot("others", "selected", sessions, false)).toMatchObject({
      targetIds: ["hidden", "primary"],
      eligible: true,
    });
    expect(buildBulkSessionRemovalSnapshot("all", "selected", sessions, false)).toMatchObject({
      targetIds: ["hidden", "primary", "selected"],
      eligible: true,
    });
  });

  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.3 refuses loading, empty, and active targets", () => {
    expect(
      buildBulkSessionRemovalSnapshot(
        "others",
        "selected",
        [taskSession("selected", "COMPLETED")],
        false,
      ),
    ).toMatchObject({ eligible: false, reason: "empty" });
    expect(
      buildBulkSessionRemovalSnapshot(
        "all",
        "selected",
        [taskSession("selected", "COMPLETED")],
        true,
      ),
    ).toMatchObject({ eligible: false, reason: "loading" });
    expect(
      buildBulkSessionRemovalSnapshot(
        "all",
        "selected",
        [taskSession("selected", "RUNNING")],
        false,
      ),
    ).toMatchObject({ eligible: false, reason: "active" });
    expect(
      buildBulkSessionRemovalSnapshot(
        "all",
        "selected",
        [taskSession("selected", "COMPLETED")],
        false,
        true,
      ),
    ).toMatchObject({ eligible: false, reason: "error" });
  });

  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.3 invalidates a snapshot after an eligible target changes", () => {
    const snapshot = buildBulkSessionRemovalSnapshot(
      "others",
      "selected",
      [taskSession("selected", "COMPLETED"), taskSession("other", "COMPLETED")],
      false,
    );

    expect(
      isBulkSessionRemovalSnapshotCurrent(
        snapshot,
        [taskSession("selected", "COMPLETED"), taskSession("other", "RUNNING")],
        false,
      ),
    ).toBe(false);
  });
});

describe("bulk session removal execution", () => {
  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.5 removes sequentially and leaves a selected other session", async () => {
    const remove = vi.fn<(id: string) => Promise<boolean>>().mockResolvedValue(true);
    const result = await executeBulkSessionRemoval(["hidden", "primary"], remove);

    expect(remove.mock.calls.map((call) => call[0])).toEqual(["hidden", "primary"]);
    expect(result).toEqual({ removed: 2, remaining: 0, failed: false });
  });

  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.6 stops at the first failure", async () => {
    const remove = vi
      .fn<(id: string) => Promise<boolean>>()
      .mockImplementation(async (id) => id !== "second");
    const result = await executeBulkSessionRemoval(["first", "second", "third"], remove);

    expect(remove.mock.calls.map((call) => call[0])).toEqual(["first", "second"]);
    expect(result).toEqual({ removed: 1, remaining: 2, failed: true });
  });

  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.6 rejects a second submit while deletion is pending", async () => {
    let finishFirst: (value: boolean) => void = () => undefined;
    const first = new Promise<boolean>((resolve) => {
      finishFirst = resolve;
    });
    const remove = vi.fn<(id: string) => Promise<boolean>>().mockReturnValueOnce(first);
    const sessions = [taskSession("first", "COMPLETED"), taskSession("second", "COMPLETED")];
    const hook = renderHook(() => useBulkSessionRemoval({ sessions, isLoading: false, remove }));

    act(() => hook.result.current.request("all", "first"));
    let submission: ReturnType<typeof hook.result.current.confirm>;
    act(() => {
      submission = hook.result.current.confirm();
    });
    await act(async () => {
      expect(await hook.result.current.confirm()).toBeNull();
    });
    expect(remove).toHaveBeenCalledTimes(1);
    await act(async () => {
      finishFirst(false);
      await submission;
    });
    expect(remove).toHaveBeenCalledTimes(1);
  });

  it("@covers AC-TASKS-BULK-SESSION-REMOVAL-001.3 refreshes stale Remove All before deleting the promoted primary", async () => {
    const remove = vi.fn<(id: string) => Promise<boolean>>().mockResolvedValue(true);
    const primary = taskSession("primary", "CREATED", true);
    const deletedSibling = taskSession("deleted-sibling", "COMPLETED");
    const hook = renderHook(
      ({ sessions }) => useBulkSessionRemoval({ sessions, isLoading: false, remove }),
      { initialProps: { sessions: [primary, deletedSibling] } },
    );

    act(() => hook.result.current.request("all", "primary"));
    hook.rerender({ sessions: [primary] });

    await act(async () => {
      expect(await hook.result.current.confirm()).toEqual({ stale: true });
    });
    expect(remove).not.toHaveBeenCalled();
    expect(hook.result.current.wasRefreshed).toBe(true);
    expect(hook.result.current.snapshot?.targetIds).toEqual(["primary"]);

    await act(async () => {
      expect(await hook.result.current.confirm()).toEqual({
        stale: false,
        removed: 1,
        remaining: 0,
        failed: false,
      });
    });
    expect(remove).toHaveBeenCalledWith("primary");
  });
});
