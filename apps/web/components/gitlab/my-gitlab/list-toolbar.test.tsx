import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ListToolbar } from "./list-toolbar";

afterEach(() => cleanup());

const MILESTONE_TEST_ID = "gitlab-milestone-filter";

function renderToolbar({
  milestone = "",
  committedMilestone = "",
}: { milestone?: string; committedMilestone?: string } = {}) {
  const onMilestoneChange = vi.fn();
  const onCommitMilestone = vi.fn();
  render(
    <TooltipProvider>
      <ListToolbar
        title="Assigned to me"
        count={2}
        loading={false}
        lastFetchedAt={null}
        customQuery=""
        committedQuery=""
        onCustomQueryChange={vi.fn()}
        onCommitCustomQuery={vi.fn()}
        projectFilter=""
        onProjectFilterChange={vi.fn()}
        projectOptions={[]}
        onRefresh={vi.fn()}
        showMilestoneFilter
        milestone={milestone}
        committedMilestone={committedMilestone}
        onMilestoneChange={onMilestoneChange}
        onCommitMilestone={onCommitMilestone}
      />
    </TooltipProvider>,
  );
  return { onMilestoneChange, onCommitMilestone };
}

describe("MilestoneFilterInput", () => {
  it("does not render when showMilestoneFilter is false", () => {
    render(
      <TooltipProvider>
        <ListToolbar
          title="Assigned to me"
          count={2}
          loading={false}
          lastFetchedAt={null}
          customQuery=""
          committedQuery=""
          onCustomQueryChange={vi.fn()}
          onCommitCustomQuery={vi.fn()}
          projectFilter=""
          onProjectFilterChange={vi.fn()}
          projectOptions={[]}
          onRefresh={vi.fn()}
          showMilestoneFilter={false}
          milestone=""
          committedMilestone=""
          onMilestoneChange={vi.fn()}
          onCommitMilestone={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.queryByTestId(MILESTONE_TEST_ID)).toBeNull();
  });

  it("commits on blur when the draft differs from the committed milestone", () => {
    const { onCommitMilestone } = renderToolbar({ milestone: "Next", committedMilestone: "" });

    fireEvent.blur(screen.getByTestId(MILESTONE_TEST_ID));

    expect(onCommitMilestone).toHaveBeenCalledTimes(1);
  });

  it("does not commit on blur when the draft matches the committed milestone", () => {
    const { onCommitMilestone } = renderToolbar({ milestone: "Next", committedMilestone: "Next" });

    fireEvent.blur(screen.getByTestId(MILESTONE_TEST_ID));

    expect(onCommitMilestone).not.toHaveBeenCalled();
  });

  it("commits on Enter and prevents the default form submission", () => {
    const { onCommitMilestone } = renderToolbar({ milestone: "Next", committedMilestone: "" });

    const input = screen.getByTestId(MILESTONE_TEST_ID);
    const event = fireEvent.keyDown(input, { key: "Enter" });

    expect(onCommitMilestone).toHaveBeenCalledTimes(1);
    expect(event).toBe(false);
  });

  it("does not commit while an IME is composing a milestone candidate", () => {
    const { onCommitMilestone } = renderToolbar({ milestone: "次", committedMilestone: "" });

    fireEvent.keyDown(screen.getByTestId(MILESTONE_TEST_ID), {
      key: "Enter",
      isComposing: true,
    });

    expect(onCommitMilestone).not.toHaveBeenCalled();
  });

  it("does not commit on keys other than Enter", () => {
    const { onCommitMilestone } = renderToolbar({ milestone: "Next", committedMilestone: "" });

    fireEvent.keyDown(screen.getByTestId(MILESTONE_TEST_ID), { key: "Escape" });

    expect(onCommitMilestone).not.toHaveBeenCalled();
  });

  it("reports every keystroke through onMilestoneChange without committing", () => {
    const { onMilestoneChange, onCommitMilestone } = renderToolbar();

    fireEvent.change(screen.getByTestId(MILESTONE_TEST_ID), { target: { value: "Sprint 42" } });

    expect(onMilestoneChange).toHaveBeenCalledWith("Sprint 42");
    expect(onCommitMilestone).not.toHaveBeenCalled();
  });
});
