import { create } from "zustand";
import type { TaskWalkthrough } from "@/lib/types/http";

type Draft = { sessionId: string; taskId: string; revision: string; text: string };
type DraftState = {
  drafts: Record<string, Draft>;
  setDraft: (
    sessionId: string,
    walkthrough: TaskWalkthrough,
    stepIndex: number,
    text: string,
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
  setDraft: (sessionId, walkthrough, stepIndex, text) =>
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
      if (text) drafts[key] = { sessionId, taskId: walkthrough.task_id, revision, text };
      else delete drafts[key];
      return { drafts };
    }),
  clearSession: (sessionId) =>
    set((state) => ({
      drafts: Object.fromEntries(
        Object.entries(state.drafts).filter(([, draft]) => draft.sessionId !== sessionId),
      ),
    })),
}));
