import { act, renderHook } from "@testing-library/react";
import { useEffect, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getChatDraftAttachments,
  getChatDraftText,
  setChatDraftAttachments,
  setChatDraftText,
} from "@/lib/local-storage";
import type {
  ChatSubmitPayload,
  ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import type { QuickChatInitialPrompt } from "@/lib/state/slices/ui/types";
import { useQuickChatInitialPrompt } from "./use-quick-chat-initial-prompt";

const LAUNCH_PROMPT = "Start here";
const SESSION_ID = "session-1";
const TRACE_FILE_NAME = "trace.txt";
const MANUAL_FOLLOW_UP = "manual follow-up";
const TEXT_MIME_TYPE = "text/plain";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("useQuickChatInitialPrompt draft recovery", () => {
  it.each([false, true])(
    "keeps a rejected launch as a manual draft without resending on remount (throws=%s)",
    async (throws) => {
      let pending: string | undefined = LAUNCH_PROMPT;
      const submit = throws
        ? vi.fn().mockRejectedValue(new Error("connection lost"))
        : vi.fn().mockResolvedValue(false);
      const onAttempted = () => {
        pending = undefined;
      };
      const onRejected = vi.fn();
      const mount = () =>
        renderHook(() =>
          useQuickChatInitialPrompt({
            sessionId: SESSION_ID,
            taskId: "task-1",
            prompt: pending,
            blocked: false,
            submit,
            onAttempted,
            onRejected,
          }),
        );
      const first = mount();
      await act(async () => {});
      first.unmount();
      const second = mount();
      await act(async () => {});

      expect(submit).toHaveBeenCalledTimes(1);
      expect(getChatDraftText(SESSION_ID)).toBe(LAUNCH_PROMPT);
      expect(onRejected).toHaveBeenCalledWith(SESSION_ID, LAUNCH_PROMPT);
      second.unmount();
    },
  );

  it("preserves a newer manual draft when automatic delivery settles", async () => {
    let accept!: (value: boolean) => void;
    const submit = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          accept = resolve;
        }),
    );
    renderHook(() =>
      useQuickChatInitialPrompt({
        sessionId: SESSION_ID,
        taskId: "task-1",
        prompt: LAUNCH_PROMPT,
        blocked: false,
        submit,
      }),
    );
    await act(async () => {});
    const recoveryDraft = getChatDraftText(SESSION_ID);
    setChatDraftText(SESSION_ID, MANUAL_FOLLOW_UP);
    await act(async () => accept(true));
    expect(recoveryDraft).toBe(LAUNCH_PROMPT);
    expect(getChatDraftText(SESSION_ID)).toBe(MANUAL_FOLLOW_UP);
  });

  it("keeps a blocked prompt bound to its original session submitter", async () => {
    const firstSubmit = vi.fn().mockResolvedValue(true);
    const nextSubmit = vi.fn().mockResolvedValue(true);
    const view = renderHook(
      ({ sessionId, submit, blocked }) =>
        useQuickChatInitialPrompt({
          sessionId,
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          submit,
          blocked,
        }),
      { initialProps: { sessionId: SESSION_ID, submit: firstSubmit, blocked: false } },
    );
    view.rerender({ sessionId: "session-2", submit: nextSubmit, blocked: true });
    await act(async () => {});

    expect(firstSubmit).not.toHaveBeenCalled();
    expect(nextSubmit).not.toHaveBeenCalled();

    view.rerender({ sessionId: SESSION_ID, submit: firstSubmit, blocked: false });
    await act(async () => {});
    expect(firstSubmit).toHaveBeenCalledOnce();
    expect(nextSubmit).not.toHaveBeenCalled();
  });
});

describe("useQuickChatInitialPrompt session identity", () => {
  it("does not hand session A's scheduled payload or callback to unblocked session B", async () => {
    const submitA = vi.fn().mockResolvedValue(true);
    const submitB = vi.fn().mockResolvedValue(true);
    const attemptedA = vi.fn();
    const attemptedB = vi.fn();
    const acceptedB = vi.fn();
    const payloadA = { message: "A's opening message" };
    const payloadB = { message: "B's pending opening message" };
    let pendingB: typeof payloadB | undefined = payloadB;
    type HandoffProps = {
      sessionId: string;
      taskId: string;
      prompt?: QuickChatInitialPrompt;
      submit: (payload: ChatSubmitPayload) => ChatSubmitResult;
      onAttempted?: () => void;
      onAccepted?: (sessionId: string, prompt: QuickChatInitialPrompt) => void;
    };
    const initialProps: HandoffProps = {
      sessionId: "session-A",
      taskId: "task-A",
      prompt: payloadA,
      submit: submitA,
      onAttempted: () => attemptedA(),
      onAccepted: () => undefined,
    };
    const view = renderHook(
      ({ sessionId, taskId, prompt, submit, onAttempted, onAccepted }: HandoffProps) =>
        useQuickChatInitialPrompt({
          sessionId,
          taskId,
          prompt,
          blocked: false,
          submit,
          onAttempted,
          onAccepted,
        }),
      { initialProps },
    );

    // Rerender before the scheduled promise microtask flushes.
    view.rerender({
      sessionId: "session-B",
      taskId: "task-B",
      prompt: pendingB,
      submit: submitB,
      onAttempted: () => {
        attemptedB();
        pendingB = undefined;
      },
      onAccepted: acceptedB,
    });
    await act(async () => {});

    expect(submitA).not.toHaveBeenCalled();
    expect(submitB).toHaveBeenCalledOnce();
    expect(submitB).toHaveBeenCalledWith(payloadB);
    expect(attemptedA).not.toHaveBeenCalled();
    expect(attemptedB).toHaveBeenCalledOnce();
    expect(acceptedB).toHaveBeenCalledWith("session-B", payloadB);
    expect(pendingB).toBeUndefined();
    expect(getChatDraftText("session-A")).toBe(payloadA.message);
    expect(getChatDraftText("session-B")).toBe("");
  });
});

describe("useQuickChatInitialPrompt admission", () => {
  it("rechecks prerequisites after an earlier passive effect blocks admission", async () => {
    const submit = vi.fn().mockResolvedValue(true);
    const onAttempted = vi.fn();
    const view = renderHook(() => {
      const [blocked, setBlocked] = useState(false);
      useEffect(() => setBlocked(true), []);
      useQuickChatInitialPrompt({
        sessionId: SESSION_ID,
        taskId: "task-1",
        prompt: LAUNCH_PROMPT,
        blocked,
        submit,
        onAttempted,
      });
      return setBlocked;
    });

    await act(async () => {});
    expect(submit).not.toHaveBeenCalled();
    expect(onAttempted).not.toHaveBeenCalled();

    act(() => view.result.current(false));
    await act(async () => {});
    expect(submit).toHaveBeenCalledOnce();
    expect(onAttempted).toHaveBeenCalledOnce();
  });

  it("waits for admission prerequisites and submits once they are ready", async () => {
    const submit = vi.fn().mockResolvedValue(true);
    const view = renderHook(
      ({ blocked }) =>
        useQuickChatInitialPrompt({
          sessionId: SESSION_ID,
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          blocked,
          submit,
        }),
      { initialProps: { blocked: true } },
    );

    await act(async () => {});
    expect(submit).not.toHaveBeenCalled();

    view.rerender({ blocked: false });
    await act(async () => {});
    expect(submit).toHaveBeenCalledOnce();
    expect(submit).toHaveBeenCalledWith({ message: LAUNCH_PROMPT });
  });
});

describe("useQuickChatInitialPrompt rejected payload recovery", () => {
  it("submits and recovers the full opening payload without replay", async () => {
    const openingPayload = {
      message: "Review this trace",
      clientMessageId: "opening-message-1",
      attachments: [
        {
          type: "resource" as const,
          attachment_id: "attachment-1",
          mime_type: TEXT_MIME_TYPE,
          name: TRACE_FILE_NAME,
          size_bytes: 42,
          delivery_mode: "path" as const,
        },
      ],
    };
    let pending: typeof openingPayload | undefined = openingPayload;
    const submit = vi.fn().mockResolvedValue(false);
    const onRejected = vi.fn();
    const mount = () =>
      renderHook(() =>
        useQuickChatInitialPrompt({
          sessionId: SESSION_ID,
          taskId: "task-1",
          prompt: pending,
          blocked: false,
          submit,
          onAttempted: () => {
            pending = undefined;
          },
          onRejected,
        }),
      );
    const first = mount();
    await act(async () => {});
    first.unmount();
    const second = mount();
    await act(async () => {});

    expect(submit).toHaveBeenCalledOnce();
    expect(submit).toHaveBeenCalledWith(openingPayload);
    expect(getChatDraftText(SESSION_ID)).toBe(openingPayload.message);
    expect(getChatDraftAttachments(SESSION_ID)).toEqual([
      expect.objectContaining({
        attachmentId: "attachment-1",
        fileName: TRACE_FILE_NAME,
        mimeType: TEXT_MIME_TYPE,
        size: 42,
        deliveryMode: "path",
      }),
    ]);
    expect(onRejected).toHaveBeenCalledWith(SESSION_ID, openingPayload);
    second.unmount();
  });
});

describe("useQuickChatInitialPrompt accepted payload cleanup", () => {
  it("clears only the accepted opening snapshot and its matching file descriptors", async () => {
    const openingPayload = {
      message: "Review this trace",
      clientMessageId: "opening-message-2",
      attachments: [
        {
          type: "resource" as const,
          attachment_id: "attachment-2",
          mime_type: TEXT_MIME_TYPE,
          name: TRACE_FILE_NAME,
          size_bytes: 42,
          delivery_mode: "path" as const,
        },
      ],
    };
    const submit = vi.fn().mockResolvedValue(true);
    renderHook(() =>
      useQuickChatInitialPrompt({
        sessionId: SESSION_ID,
        taskId: "task-1",
        prompt: openingPayload,
        blocked: false,
        submit,
      }),
    );
    await act(async () => {});

    expect(submit).toHaveBeenCalledWith(openingPayload);
    expect(getChatDraftText(SESSION_ID)).toBe("");
    expect(getChatDraftAttachments(SESSION_ID)).toEqual([]);
  });

  it("preserves a newer text and attachment draft when the opening send settles", async () => {
    let accept!: (value: boolean) => void;
    const openingPayload = {
      message: "Review this trace",
      clientMessageId: "opening-message-3",
      attachments: [
        {
          type: "resource" as const,
          attachment_id: "attachment-3",
          mime_type: TEXT_MIME_TYPE,
          name: TRACE_FILE_NAME,
          size_bytes: 42,
          delivery_mode: "path" as const,
        },
      ],
    };
    const submit = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          accept = resolve;
        }),
    );
    renderHook(() =>
      useQuickChatInitialPrompt({
        sessionId: SESSION_ID,
        taskId: "task-1",
        prompt: openingPayload,
        blocked: false,
        submit,
      }),
    );
    await act(async () => {});
    setChatDraftText(SESSION_ID, MANUAL_FOLLOW_UP);
    setChatDraftAttachments(SESSION_ID, [
      {
        id: "new-file",
        attachmentId: "attachment-new",
        mimeType: TEXT_MIME_TYPE,
        fileName: "follow-up.txt",
        size: 18,
        isImage: false,
        deliveryMode: "path",
      },
    ]);

    await act(async () => accept(true));

    expect(getChatDraftText(SESSION_ID)).toBe(MANUAL_FOLLOW_UP);
    expect(getChatDraftAttachments(SESSION_ID)).toEqual([
      expect.objectContaining({ attachmentId: "attachment-new", fileName: "follow-up.txt" }),
    ]);
  });
});

describe("useQuickChatInitialPrompt admission outcomes", () => {
  it("waits for admission prerequisites and clears only after acceptance", async () => {
    const submit = vi.fn().mockResolvedValue(true);
    const onAccepted = vi.fn();
    const view = renderHook(
      ({ blocked }) =>
        useQuickChatInitialPrompt({
          sessionId: SESSION_ID,
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          blocked,
          submit,
          onAccepted,
        }),
      { initialProps: { blocked: true } },
    );

    expect(submit).not.toHaveBeenCalled();
    view.rerender({ blocked: false });
    await act(async () => {});

    expect(submit).toHaveBeenCalledWith({ message: LAUNCH_PROMPT });
    expect(onAccepted).toHaveBeenCalledTimes(1);
  });

  it("preserves the launch prompt when delivery is rejected", async () => {
    const submit = vi.fn().mockResolvedValue(false);
    const onAccepted = vi.fn();
    renderHook(() =>
      useQuickChatInitialPrompt({
        sessionId: SESSION_ID,
        taskId: "task-1",
        prompt: LAUNCH_PROMPT,
        blocked: false,
        submit,
        onAccepted,
      }),
    );
    await act(async () => {});

    expect(onAccepted).not.toHaveBeenCalled();
  });

  it("does not retry a rejected prompt when callback identities change", async () => {
    const firstSubmit = vi.fn().mockResolvedValue(false);
    const secondSubmit = vi.fn().mockResolvedValue(false);
    const view = renderHook(
      ({ submit }: { submit: (payload: ChatSubmitPayload) => ChatSubmitResult }) =>
        useQuickChatInitialPrompt({
          sessionId: SESSION_ID,
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          blocked: false,
          submit,
        }),
      { initialProps: { submit: firstSubmit } },
    );
    await act(async () => {});

    view.rerender({ submit: secondSubmit });
    await act(async () => {});

    expect(firstSubmit).toHaveBeenCalledTimes(1);
    expect(secondSubmit).not.toHaveBeenCalled();
  });

  it("does not retry a synchronously rejected prompt", async () => {
    const firstSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult = vi.fn(
      (_payload: ChatSubmitPayload) => {
        throw new Error("rejected before returning a promise");
      },
    );
    const secondSubmit = vi.fn().mockResolvedValue(false);
    const view = renderHook(
      ({ submit }: { submit: (payload: ChatSubmitPayload) => ChatSubmitResult }) =>
        useQuickChatInitialPrompt({
          sessionId: SESSION_ID,
          taskId: "task-1",
          prompt: LAUNCH_PROMPT,
          blocked: false,
          submit,
        }),
      { initialProps: { submit: firstSubmit } },
    );
    await act(async () => {});

    view.rerender({ submit: secondSubmit });
    await act(async () => {});

    expect(firstSubmit).toHaveBeenCalledTimes(1);
    expect(secondSubmit).not.toHaveBeenCalled();
  });
});
