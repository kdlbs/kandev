import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  addTrigger,
  deleteTrigger,
  getAutomation,
  updateTrigger,
} from "@/lib/api/domains/automation-api";
import type { AutomationTrigger } from "@/lib/types/automation";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";

vi.mock("@/lib/api/domains/automation-api", () => ({
  addTrigger: vi.fn(),
  deleteTrigger: vi.fn(),
  getAutomation: vi.fn(),
  updateTrigger: vi.fn(),
}));

const AUTOMATION_ID = "automation-1";

function trigger(id: string, config: Record<string, unknown> = {}): AutomationTrigger {
  return {
    id,
    automation_id: AUTOMATION_ID,
    type: "scheduled",
    config,
    enabled: true,
    last_evaluated_at: null,
    created_at: "",
    updated_at: "",
  };
}

describe("useAutomationTriggerDrafts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps trigger edits local until persistDrafts", async () => {
    const { result } = renderHook(() => useAutomationTriggerDrafts(AUTOMATION_ID));
    act(() => result.current.loadTriggers([trigger("trigger-1")]));

    await act(() => result.current.handleToggle("trigger-1", false));
    await act(() => result.current.handleAdd("webhook", { filter_expression: "main" }));

    expect(updateTrigger).not.toHaveBeenCalled();
    expect(addTrigger).not.toHaveBeenCalled();
  });

  it("does not recreate completed trigger drafts when a save is retried", async () => {
    const first = trigger("created-1");
    const second = trigger("created-2");
    vi.mocked(addTrigger)
      .mockResolvedValueOnce(first)
      .mockRejectedValueOnce(new Error("temporary failure"))
      .mockResolvedValueOnce(second);
    vi.mocked(getAutomation).mockResolvedValue({ triggers: [first, second] } as never);
    const { result } = renderHook(() => useAutomationTriggerDrafts(AUTOMATION_ID));

    await act(() => result.current.handleAdd("webhook", { order: 1 }));
    await act(() => result.current.handleAdd("webhook", { order: 2 }));
    let saveError: unknown;
    await act(async () => {
      try {
        await result.current.persistDrafts();
      } catch (error) {
        saveError = error;
      }
    });
    expect(saveError).toEqual(new Error("temporary failure"));
    await act(() => result.current.persistDrafts());

    expect(addTrigger).toHaveBeenCalledTimes(3);
    expect(addTrigger).toHaveBeenNthCalledWith(
      3,
      expect.objectContaining({ config: { order: 2 } }),
    );
    expect(deleteTrigger).not.toHaveBeenCalled();
  });

  it("preserves a trigger edit made while save is in flight", async () => {
    let finishUpdate: () => void = () => undefined;
    vi.mocked(updateTrigger).mockImplementation(
      () =>
        new Promise((resolve) => {
          finishUpdate = () => resolve({ updated: true });
        }),
    );
    vi.mocked(getAutomation).mockResolvedValue({
      triggers: [{ ...trigger("trigger-1"), enabled: false }],
    } as never);
    const { result } = renderHook(() => useAutomationTriggerDrafts(AUTOMATION_ID));
    act(() => result.current.loadTriggers([trigger("trigger-1")]));
    await act(() => result.current.handleToggle("trigger-1", false));

    let savePromise!: Promise<AutomationTrigger[]>;
    act(() => {
      savePromise = result.current.persistDrafts();
    });
    await act(() => result.current.handleToggle("trigger-1", true));
    await act(async () => {
      finishUpdate();
      await savePromise;
    });

    expect(result.current.allTriggers[0].enabled).toBe(true);
  });
});
