import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

import { PreviewFeedbackControls } from "./preview-feedback-controls";

function controller(overrides: Record<string, unknown> = {}) {
  return {
    items: [{ id: "a" }, { id: "b" }],
    drafts: [],
    candidateLabel: null,
    acknowledgedMode: null,
    annotation: { active: false, toggle: vi.fn() },
    ...overrides,
  } as unknown as PreviewCaptureController;
}

afterEach(cleanup);

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
describe("PreviewFeedbackControls", () => {
  it("toggles annotation mode and shows the pending count", () => {
    const capture = controller();
    render(<PreviewFeedbackControls capture={capture} enabled />);

    const toggle = screen.getByTestId("preview-feedback-trigger");
    expect(toggle.textContent).toContain("2");
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(toggle);
    expect(capture.annotation.toggle).toHaveBeenCalled();
  });

  it("reflects the active mode and flags unsaved drafts", () => {
    render(
      <PreviewFeedbackControls
        capture={controller({
          annotation: { active: true, toggle: vi.fn() },
          drafts: [{ id: "d1" }],
        })}
        enabled
      />,
    );

    expect(screen.getByTestId("preview-feedback-trigger").getAttribute("aria-pressed")).toBe(
      "true",
    );
    expect(screen.getByTestId("preview-feedback-unsaved-indicator")).toBeTruthy();
  });

  it("is disabled without a task and session", () => {
    render(<PreviewFeedbackControls capture={controller()} enabled={false} />);
    expect(screen.getByTestId("preview-feedback-trigger").hasAttribute("disabled")).toBe(true);
  });
});
