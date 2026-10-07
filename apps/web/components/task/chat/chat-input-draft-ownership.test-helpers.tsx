import { createRef, forwardRef, useImperativeHandle, useLayoutEffect, type ReactNode } from "react";
import { act, render } from "@testing-library/react";
import { expect, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  getChatDraftAttachments,
  getChatDraftContent,
  getChatDraftText,
  setChatDraftContent,
  setChatDraftText,
} from "@/lib/local-storage";
import type { ReviewComment } from "@/lib/state/slices/comments";
import { ChatInputContainer } from "./chat-input-container";
import { TipTapInput } from "./tiptap-input";
import { useChatInputState } from "./use-chat-input-state";

export type DraftState = ReturnType<typeof useChatInputState>;
export type Submit = Parameters<typeof useChatInputState>[0]["onSubmit"];
// i18n-exempt: Synthetic draft text used only by the real-editor test fixture.
export const MESSAGE = "Check the pending edits";
export const FIRST = "draft-owner-first";
export const SECOND = "draft-owner-second";

function documentFor(text: string) {
  return { type: "doc", content: [{ type: "paragraph", content: [{ type: "text", text }] }] };
}

export function saveDraft(sessionId: string) {
  setChatDraftText(sessionId, MESSAGE);
  setChatDraftContent(sessionId, documentFor(MESSAGE));
}

export function savedDraft(sessionId: string) {
  return {
    text: getChatDraftText(sessionId),
    content: getChatDraftContent(sessionId),
    attachments: getChatDraftAttachments(sessionId),
  };
}

export function pendingSend() {
  let accept!: (result: boolean) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<boolean>((resolve, fail) => {
    accept = resolve;
    reject = fail;
  });
  return { promise, accept, reject };
}

export function Providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider initialState={{ prompts: { items: [], loaded: true, loading: false } }}>
      <ToastProvider>
        <TooltipProvider>{children}</TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

type ComposerProps = {
  sessionId: string | null;
  onSubmit: Submit;
  taskId?: string;
  onCommit?: (state: DraftState) => void;
  pendingCommentsByFile?: Record<string, ReviewComment[]>;
  suspend?: Promise<boolean>;
};

export const Composer = forwardRef<DraftState, ComposerProps>(function Composer(
  { sessionId, onSubmit, taskId = "draft-owner-task", onCommit, pendingCommentsByFile, suspend },
  ref,
) {
  const state = useChatInputState({
    sessionId,
    taskId,
    isSending: false,
    contextItems: [],
    showRequestChangesTooltip: false,
    onSubmit,
    pendingCommentsByFile,
  });
  useImperativeHandle(ref, () => state);
  useLayoutEffect(() => onCommit?.(state), [onCommit]);
  if (suspend) throw suspend;
  return (
    <TipTapInput
      ref={state.inputRef}
      sessionId={sessionId}
      taskId={taskId}
      value={state.value}
      onChange={state.handleChange}
      onSubmit={() => state.handleSubmit(() => {})}
      entityReferencesEnabled={false}
    />
  );
});

export function mountComposer(sessionId: string | null, onSubmit: Submit) {
  const ref = createRef<DraftState>();
  const tree = (id: string | null, key = "retained", taskId = "draft-owner-task") => (
    <Providers>
      <Composer ref={ref} key={key} sessionId={id} taskId={taskId} onSubmit={onSubmit} />
    </Providers>
  );
  const view = render(tree(sessionId));
  return { ref, tree, view };
}

export async function flushEditor() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(80);
  });
}

export async function settle(send: ReturnType<typeof pendingSend>, accepted = true) {
  await act(async () => {
    send.accept(accepted);
    await send.promise;
  });
  await flushEditor();
}

export function expectEditor(ref: ReturnType<typeof createRef<DraftState>>, text = MESSAGE) {
  expect(ref.current?.value).toBe(text);
  expect(ref.current?.inputRef.current?.getValue()).toBe(text);
  expect(ref.current?.inputRef.current?.getTextareaElement()?.textContent).toBe(text);
}

export function Container({ sessionId, onSubmit }: ComposerProps) {
  return (
    <ChatInputContainer
      sessionId={sessionId}
      taskId={null}
      onSubmit={onSubmit}
      taskDescription=""
      planModeEnabled={false}
      onPlanModeChange={() => {}}
      isAgentBusy={false}
      isWorking={false}
      isStarting={false}
      isSending={false}
      onCancel={() => {}}
      hideSessionsDropdown
      hideAgentControls
      hidePlanMode
      minimalToolbar
    />
  );
}
