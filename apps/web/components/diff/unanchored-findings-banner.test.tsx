import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { TaskReviewFinding } from "@/lib/types/review";
import { NAVIGATE_FINDING_EVENT } from "@/lib/review/navigation";
import { UnanchoredFindingsBanner } from "./unanchored-findings-banner";

// The real card wires store-backed actions; the banner's own logic (open-filter
// + auto-expand) is what this test covers, so stub the child.
vi.mock("./inline-review-finding", () => ({
  InlineReviewFinding: ({ finding }: { finding: TaskReviewFinding }) => (
    <div data-testid="inline-finding" data-id={finding.id} />
  ),
}));

function finding(overrides: Partial<TaskReviewFinding> = {}): TaskReviewFinding {
  return {
    id: "s1",
    run_id: "r1",
    task_id: "t1",
    repository_id: "",
    repository_name: "",
    file_path: "a.ts",
    start_line: 1,
    end_line: 1,
    side: "additions",
    severity: "major",
    category: "correctness",
    title: "stale finding",
    body: "body",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "h",
    status: "open",
    created_at: "2026-07-24T10:00:00Z",
    updated_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

const TOGGLE_ID = "unanchored-findings-toggle";
// Must match the data-testid on the stub above; the mock factory is hoisted so
// it can't reference this constant, hence the literal there.
const INLINE_FINDING_ID = "inline-finding";

function dispatchNavigate(findingId: string) {
  act(() => {
    window.dispatchEvent(new CustomEvent(NAVIGATE_FINDING_EVENT, { detail: { findingId } }));
  });
}

/** Reads the toggle's aria-expanded state ("true" / "false"). */
function expandedState() {
  return screen.getByTestId(TOGGLE_ID).getAttribute("aria-expanded");
}

afterEach(cleanup);

describe("UnanchoredFindingsBanner", () => {
  it("renders nothing when there are no open findings", () => {
    const { container } = render(
      <UnanchoredFindingsBanner findings={[finding({ status: "resolved" })]} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("is collapsed by default", () => {
    render(<UnanchoredFindingsBanner findings={[finding()]} />);
    expect(expandedState()).toBe("false");
    expect(screen.queryByTestId(INLINE_FINDING_ID)).toBeNull();
  });

  it("expands and renders its cards on click", () => {
    render(<UnanchoredFindingsBanner findings={[finding()]} />);
    fireEvent.click(screen.getByTestId(TOGGLE_ID));
    expect(expandedState()).toBe("true");
    expect(screen.getByTestId(INLINE_FINDING_ID)).toBeTruthy();
  });

  it("auto-expands when a navigate event targets one of its open findings", () => {
    render(<UnanchoredFindingsBanner findings={[finding({ id: "s1" })]} />);
    dispatchNavigate("s1");
    expect(expandedState()).toBe("true");
    expect(screen.getByTestId(INLINE_FINDING_ID).getAttribute("data-id")).toBe("s1");
  });

  it("ignores a navigate event for a finding it does not own", () => {
    render(<UnanchoredFindingsBanner findings={[finding({ id: "s1" })]} />);
    dispatchNavigate("not-mine");
    expect(expandedState()).toBe("false");
    expect(screen.queryByTestId(INLINE_FINDING_ID)).toBeNull();
  });
});
