import React, { createRef } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { shouldShowChatFocusHint, useChatInputContainer } from "./use-chat-input-container";
import type { ChatInputContainerHandle } from "./chat-input-container";

const callerPlaceholder = "Continue working on the task...";

function renderInputState(overrides: Partial<Parameters<typeof useChatInputContainer>[0]> = {}) {
  return renderHook(
    (currentOverrides: Partial<Parameters<typeof useChatInputContainer>[0]>) =>
      useChatInputContainer({
        ref: createRef<ChatInputContainerHandle>(),
        sessionId: "session-1",
        isSending: false,
        isStarting: false,
        isPreparingEnvironment: false,
        isMoving: false,
        isFailed: false,
        needsRecovery: false,
        executorUnavailable: false,
        isAgentBusy: false,
        supportsSteering: false,
        hasAgentCommands: true,
        placeholder: undefined,
        contextItems: [],
        pendingClarification: null,
        onClarificationResolved: undefined,
        pendingCommentsByFile: undefined,
        hasContextComments: false,
        showRequestChangesTooltip: false,
        onRequestChangesTooltipDismiss: undefined,
        onSubmit: vi.fn(),
        ...currentOverrides,
      }),
    {
      initialProps: overrides,
      wrapper: ({ children }) => React.createElement(ToastProvider, null, children),
    },
  );
}

describe("useChatInputContainer", () => {
  it("@covers AC-UI-SESSION-START-COMPOSER-READINESS-001.1 keeps editing available while startup blocks submission", () => {
    const { result } = renderInputState({ isStarting: true });

    expect(result.current.isDisabled).toBe(false);
    expect(result.current.submitDisabled).toBe(true);
    expect(result.current.submitDisabledReason).toBeUndefined();
  });

  it("preserves draft text when startup transitions to failed recovery", () => {
    const { result, rerender } = renderInputState({ isStarting: true });

    act(() => {
      result.current.handleChange("my draft");
    });

    rerender({ isStarting: false, isFailed: true });

    expect(result.current.value).toBe("my draft");
    expect(result.current.isDisabled).toBe(true);
    expect(result.current.submitDisabled).toBe(true);
  });

  it("surfaces the setup tooltip only while a container/sandbox is preparing", () => {
    const { result } = renderInputState({
      isStarting: true,
      isPreparingEnvironment: true,
    });

    expect(result.current.submitDisabledReason).toBe("The agent is still being set up.");
  });

  it("keeps queueing enabled for an interactive clarification during stale startup", () => {
    const { result } = renderInputState({
      isStarting: true,
      isPreparingEnvironment: true,
      placeholder: "Queue instructions while the question is pending...",
      pendingClarification: { id: "clarification-1" } as never,
      onClarificationResolved: vi.fn(),
    });

    expect(result.current.isDisabled).toBe(false);
    expect(result.current.submitDisabled).toBe(false);
    expect(result.current.submitDisabledReason).toBeUndefined();
    expect(result.current.inputPlaceholder).toBe(
      "Queue instructions while the question is pending...",
    );
  });

  it("disables submit while a staged attachment upload is pending", async () => {
    const originalFetch = globalThis.fetch;
    let resolveUpload!: (response: Response) => void;
    const uploadResponse = new Promise<Response>((resolve) => {
      resolveUpload = resolve;
    });
    globalThis.fetch = vi.fn(() => uploadResponse) as typeof globalThis.fetch;
    try {
      const { result } = renderInputState({ workspaceId: "workspace-1" });
      await act(async () => {
        await result.current.addFiles([
          new File([new Uint8Array(6 * 1024 * 1024)], "large.bin", {
            type: "application/octet-stream",
          }),
        ]);
      });

      await waitFor(() => expect(result.current.hasPendingAttachmentUploads).toBe(true));
      expect(result.current.isDisabled).toBe(false);
      expect(result.current.submitDisabled).toBe(true);

      resolveUpload(
        new Response(
          JSON.stringify({
            attachment_id: "attachment-1",
            name: "large.bin",
            mime_type: "application/octet-stream",
            kind: "resource",
            delivery_mode: "path",
            size_bytes: 6 * 1024 * 1024,
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        ),
      );
      await waitFor(() => expect(result.current.hasPendingAttachmentUploads).toBe(false));
      expect(result.current.submitDisabled).toBe(false);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it("shows the steer affordance over a caller placeholder when the session can steer", () => {
    const { result } = renderInputState({
      supportsSteering: true,
      placeholder: callerPlaceholder,
    });

    // The steer label must win: a send here is delivered into the running turn,
    // and the generic "Continue working…" prompt would mask that. Assert the
    // resolved chat:composerSteerPlaceholder itself — asserting only that it
    // differs from the caller's would also pass on any other generic label.
    expect(result.current.inputPlaceholder).toBe("Send now; delivered to the running turn");
  });

  it("keeps the caller placeholder when the session cannot steer", () => {
    const { result } = renderInputState({
      supportsSteering: false,
      placeholder: callerPlaceholder,
    });

    expect(result.current.inputPlaceholder).toBe(callerPlaceholder);
  });
});

describe("shouldShowChatFocusHint", () => {
  it("hides the hint when a blurred editor has draft text", () => {
    expect(
      shouldShowChatFocusHint({
        isInputFocused: false,
        value: "halle from chat input",
        hasClarification: false,
        hasPendingComments: false,
      }),
    ).toBe(false);
  });

  it("shows the hint only for an empty blurred editor without blocking overlays", () => {
    expect(
      shouldShowChatFocusHint({
        isInputFocused: false,
        value: "   ",
        hasClarification: false,
        hasPendingComments: false,
      }),
    ).toBe(true);
    expect(
      shouldShowChatFocusHint({
        isInputFocused: true,
        value: "",
        hasClarification: false,
        hasPendingComments: false,
      }),
    ).toBe(false);
  });
});
