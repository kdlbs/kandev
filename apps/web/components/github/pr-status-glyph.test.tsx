import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { PRCIState } from "@/lib/types/github";
import { PRStatusGlyph } from "./pr-status-glyph";

afterEach(() => cleanup());

const CI_TEST_IDS: Record<Exclude<PRCIState, "none">, string> = {
  failed: "pr-ci-bubble-failed",
  approval_required: "pr-ci-bubble-approval-required",
  attention: "pr-ci-bubble-attention",
  in_progress: "pr-ci-bubble-in-progress",
};

const APPROVAL_BUBBLE = "pr-ci-bubble-approval-required";

describe("PRStatusGlyph", () => {
  it("renders no upper-right element for none", () => {
    const { container } = render(<PRStatusGlyph ciState="none" />);
    for (const testId of Object.values(CI_TEST_IDS)) {
      expect(screen.queryByTestId(testId)).toBeNull();
    }
    expect(container.querySelector(".absolute.-right-1")).toBeNull();
  });

  it.each(Object.entries(CI_TEST_IDS))("renders the %s bubble", (state, testId) => {
    render(<PRStatusGlyph ciState={state as Exclude<PRCIState, "none">} />);
    expect(screen.getByTestId(testId)).not.toBeNull();
  });

  it("uses the shared amber token for approval and attention", () => {
    render(<PRStatusGlyph ciState="approval_required" />);
    expect(screen.getByTestId(APPROVAL_BUBBLE).className).toContain("text-[#D97706]");
    expect(screen.getByTestId(APPROVAL_BUBBLE).className).toContain("dark:text-[#FBBF24]");
  });

  it("retains the legacy approval test id", () => {
    render(<PRStatusGlyph ciState="approval_required" />);
    expect(screen.getByTestId(APPROVAL_BUBBLE).getAttribute("data-legacy-testid")).toBe(
      "pr-workflow-approval-warning",
    );
  });

  it("animates the progress ring only under motion-safe", () => {
    render(<PRStatusGlyph ciState="in_progress" />);
    const ring = screen.getByTestId("pr-ci-bubble-in-progress").firstElementChild;
    expect(ring?.className).toContain("motion-safe:animate-spin");
  });

  it("places the conflict triangle inline instead of in the CI slot", () => {
    render(<PRStatusGlyph ciState="approval_required" hasMergeConflicts />);
    const conflict = screen.getByTestId("pr-merge-conflict-warning");
    expect(conflict.className).not.toContain("absolute");
    expect(screen.getByTestId(APPROVAL_BUBBLE)).not.toBeNull();
  });

  it("keeps automation dots in their existing corners", () => {
    render(<PRStatusGlyph autoFixEnabled autoMergeEnabled />);
    expect(screen.getByTestId("pr-task-automation-auto-fix").className).toContain("left-0");
    expect(screen.getByTestId("pr-task-automation-auto-fix").className).toContain("top-0");
    expect(screen.getByTestId("pr-task-automation-auto-merge").className).toContain("right-0");
    expect(screen.getByTestId("pr-task-automation-auto-merge").className).toContain("bottom-0");
  });

  it("keeps the topbar base glyph at 16px", () => {
    render(<PRStatusGlyph size="topbar" />);
    const glyph = document.querySelector(".relative.inline-flex.shrink-0");
    expect(glyph?.className).toContain("size-4");
  });
});
