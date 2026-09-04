import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useQuickChatInitialPrompt } from "./use-quick-chat-initial-prompt";

describe("useQuickChatInitialPrompt", () => {
  it("waits for migration and clears the launch prompt only after acceptance", async () => {
    const submit = vi.fn().mockResolvedValue(true);
    const onAccepted = vi.fn();
    const view = renderHook(
      ({ blocked }) =>
        useQuickChatInitialPrompt({
          sessionId: "session-1",
          taskId: "task-1",
          prompt: "Start here",
          blocked,
          submit,
          onAccepted,
        }),
      { initialProps: { blocked: true } },
    );

    expect(submit).not.toHaveBeenCalled();
    view.rerender({ blocked: false });
    await act(async () => {});

    expect(submit).toHaveBeenCalledWith({ message: "Start here" });
    expect(onAccepted).toHaveBeenCalledTimes(1);
  });

  it("preserves the launch prompt when delivery is rejected", async () => {
    const submit = vi.fn().mockResolvedValue(false);
    const onAccepted = vi.fn();
    renderHook(() =>
      useQuickChatInitialPrompt({
        sessionId: "session-1",
        taskId: "task-1",
        prompt: "Start here",
        blocked: false,
        submit,
        onAccepted,
      }),
    );
    await act(async () => {});

    expect(onAccepted).not.toHaveBeenCalled();
  });
});
