import { create } from "zustand";
import type { MessageAttachment } from "@/lib/services/session-launch-service";
import type { TaskWalkthrough } from "@/lib/types/http";

type Draft = {
  sessionId: string;
  taskId: string;
  revision: string;
  text: string;
  attachments?: MessageAttachment[];
};
type DraftState = {
  drafts: Record<string, Draft>;
  setDraft: (
    sessionId: string,
    walkthrough: TaskWalkthrough,
    stepIndex: number,
    update: string | MessageAttachment[],
  ) => void;
  clearSession: (sessionId: string) => void;
};
export function walkthroughDraftKey(
  sessionId: string,
  walkthrough: TaskWalkthrough,
  stepIndex: number,
) {
  return JSON.stringify([
    sessionId,
    walkthrough.task_id,
    walkthrough.id,
    walkthrough.updated_at,
    stepIndex,
  ]);
}
export const useWalkthroughDrafts = create<DraftState>((set) => ({
  drafts: {},
  setDraft: (sessionId, walkthrough, stepIndex, update) =>
    set((state) => {
      const revision = JSON.stringify([walkthrough.id, walkthrough.updated_at]);
      const drafts = Object.fromEntries(
        Object.entries(state.drafts).filter(
          ([, draft]) =>
            draft.sessionId !== sessionId ||
            draft.taskId !== walkthrough.task_id ||
            draft.revision === revision,
        ),
      );
      const key = walkthroughDraftKey(sessionId, walkthrough, stepIndex);
      const previous = drafts[key];
      const text = typeof update === "string" ? update : (previous?.text ?? "");
      const attachments = typeof update === "string" ? previous?.attachments : update;
      if (text || attachments?.length) {
        drafts[key] = { sessionId, taskId: walkthrough.task_id, revision, text, attachments };
      } else delete drafts[key];
      return { drafts };
    }),
  clearSession: (sessionId) =>
    set((state) => ({
      drafts: Object.fromEntries(
        Object.entries(state.drafts).filter(([, draft]) => draft.sessionId !== sessionId),
      ),
    })),
}));
