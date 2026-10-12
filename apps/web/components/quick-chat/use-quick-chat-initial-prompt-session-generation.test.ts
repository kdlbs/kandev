import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useQuickChatInitialPrompt } from "./use-quick-chat-initial-prompt";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

it("does not deliver a scheduled prompt through a replacement session submitter", async () => {
  const promptA = { message: "opening A", clientMessageId: "opening-A" };
  const promptB = { message: "opening B", clientMessageId: "opening-B" };
  const submitA = vi.fn().mockResolvedValue(true);
  const submitB = vi.fn().mockResolvedValue(true);
  const attemptedA = vi.fn();
  const attemptedB = vi.fn();
  let pendingB: typeof promptB | undefined = promptB;
  const onAttemptedB = vi.fn(() => {
    pendingB = undefined;
    attemptedB();
  });
  const view = renderHook(
    ({ sessionId, taskId, prompt, submit, onAttempted }) =>
      useQuickChatInitialPrompt({ sessionId, taskId, prompt, blocked: false, submit, onAttempted }),
    {
      initialProps: {
        sessionId: "session-A",
        taskId: "task-A",
        prompt: promptA,
        submit: submitA,
        onAttempted: attemptedA,
      },
    },
  );

  view.rerender({
    sessionId: "session-B",
    taskId: "task-B",
    prompt: pendingB,
    submit: submitB,
    onAttempted: onAttemptedB,
  });
  await act(async () => {});

  expect(submitA).not.toHaveBeenCalled();
  expect(submitB).toHaveBeenCalledOnce();
  expect(submitB).toHaveBeenCalledWith(promptB);
  expect(attemptedA).not.toHaveBeenCalled();
  expect(attemptedB).toHaveBeenCalledOnce();
  expect(pendingB).toBeUndefined();
});
