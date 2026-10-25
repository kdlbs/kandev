import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WalkthroughStepFeedback } from "./walkthrough-step-feedback";
import { useWalkthroughDrafts } from "@/lib/walkthrough-feedback-state";
import type { TaskWalkthrough } from "@/lib/types/http";
const walkthrough: TaskWalkthrough = {
  id: "tour",
  task_id: "task",
  title: "Tour",
  steps: [],
  created_at: "1",
  updated_at: "1",
  created_by: "agent",
};
const props = {
  walkthrough,
  sessionId: "session",
  activeStep: 0,
  commentCount: 0,
  onSubmit: vi.fn(() => true),
  onRun: vi.fn(() => true),
};
beforeEach(() => {
  useWalkthroughDrafts.setState({ drafts: {} });
  vi.clearAllMocks();
});
afterEach(cleanup);
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.1
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.1
it("opens only on request and restores drafts across step changes and remounts", () => {
  const view = render(<WalkthroughStepFeedback {...props} />);
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.click(screen.getByRole("button"));
  expect(document.activeElement).toBe(screen.getByRole("textbox"));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "First draft" } });
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Escape" });
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button"));
  view.rerender(<WalkthroughStepFeedback {...props} activeStep={1} />);
  fireEvent.click(screen.getByRole("button"));
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
  view.unmount();
  render(<WalkthroughStepFeedback {...props} />);
  fireEvent.click(screen.getByRole("button"));
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("First draft");
});
it("retains a rejected draft and clears only after a successful save", () => {
  props.onSubmit.mockReturnValueOnce(false);
  render(<WalkthroughStepFeedback {...props} />);
  fireEvent.click(screen.getByRole("button"));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Keep me" } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("Keep me");
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.click(screen.getByRole("button"));
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
});
it("closes disclosure when changing steps without reopening it on return", () => {
  const view = render(<WalkthroughStepFeedback {...props} />);
  fireEvent.click(screen.getByRole("button"));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Keep draft" } });
  view.rerender(<WalkthroughStepFeedback {...props} activeStep={1} />);
  expect(screen.queryByRole("textbox")).toBeNull();
  view.rerender(<WalkthroughStepFeedback {...props} />);
  expect(screen.queryByRole("textbox")).toBeNull();
});
