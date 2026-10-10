import React from "react";
import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { useChatInputState } from "./use-chat-input-state";

const KEEP_DRAFT_TEXT = "keep this draft";
const WORKSPACE_ONE = "workspace-1";
beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(cleanup);

it("submits editor changes before passive draft synchronization", async () => {
  const onSubmit = vi.fn().mockResolvedValue(false);
  function Composer() {
    const input = useChatInputState({
      sessionId: "session-1",
      taskId: "task-1",
      workspaceId: WORKSPACE_ONE,
      isSending: false,
      contextItems: [],
      showRequestChangesTooltip: false,
      onSubmit,
    });
    React.useLayoutEffect(() => {
      if (!input.value) input.handleChange(KEEP_DRAFT_TEXT);
      else input.handleSubmit(vi.fn());
    }, [input.value, input.handleChange, input.handleSubmit]);
    return null;
  }
  render(React.createElement(ToastProvider, null, React.createElement(Composer)));
  await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
  expect(onSubmit).toHaveBeenCalledWith({ message: KEEP_DRAFT_TEXT });
});
