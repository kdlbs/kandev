import { beforeEach, expect, it } from "vitest";
import type { TaskWalkthrough } from "@/lib/types/http";
import { useWalkthroughDrafts, walkthroughDraftKey } from "./walkthrough-feedback-state";
const tour: TaskWalkthrough = {
  id: "tour",
  task_id: "task",
  title: "Tour",
  steps: [],
  created_at: "1",
  updated_at: "1",
  created_by: "agent",
};
beforeEach(() => useWalkthroughDrafts.setState({ drafts: {} }));
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.1
it("keeps independent step drafts and clears only a saved draft", () => {
  const store = useWalkthroughDrafts.getState();
  store.setDraft("session", tour, 0, "First");
  store.setDraft("session", tour, 1, "Second");
  expect(
    useWalkthroughDrafts.getState().drafts[walkthroughDraftKey("session", tour, 0)]?.text,
  ).toBe("First");
  store.setDraft("session", tour, 1, "");
  expect(Object.values(useWalkthroughDrafts.getState().drafts).map((d) => d.text)).toEqual([
    "First",
  ]);
});
it("isolates tasks and sessions, evicts replaced revisions, and clears a session", () => {
  const store = useWalkthroughDrafts.getState();
  store.setDraft("one", tour, 0, "Old");
  store.setDraft("two", tour, 0, "Other session");
  store.setDraft("one", { ...tour, task_id: "other" }, 0, "Other task");
  store.setDraft("one", { ...tour, updated_at: "2" }, 0, "New");
  expect(Object.values(useWalkthroughDrafts.getState().drafts).map((d) => d.text)).toEqual([
    "Other session",
    "Other task",
    "New",
  ]);
  store.clearSession("one");
  expect(Object.values(useWalkthroughDrafts.getState().drafts).map((d) => d.text)).toEqual([
    "Other session",
  ]);
});
it("clears drafts when the owning comment session is cleared", async () => {
  const { useCommentsStore } = await import("./state/slices/comments");
  useWalkthroughDrafts.getState().setDraft("session", tour, 0, "Draft only");
  useCommentsStore.getState().clearSessionComments("session");
  expect(useWalkthroughDrafts.getState().drafts).toEqual({});
});
