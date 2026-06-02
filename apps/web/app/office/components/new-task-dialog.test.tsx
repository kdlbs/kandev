import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

import { CreateTaskButton } from "./new-task-dialog";
import type { IssueDraft } from "./new-task-draft";

const BASE_DRAFT: IssueDraft = {
  title: "",
  description: "",
  assigneeId: "",
  projectId: "",
  status: "todo",
  priority: "medium",
  showReviewer: false,
  showApprover: false,
  reviewerIds: [],
  approverIds: [],
};

function renderButton(draft: Partial<IssueDraft>) {
  return render(
    <TooltipProvider>
      <CreateTaskButton draft={{ ...BASE_DRAFT, ...draft }} submitting={false} onCreate={vi.fn()} />
    </TooltipProvider>,
  );
}

afterEach(() => cleanup());

describe("CreateTaskButton", () => {
  it("disables the button and labels missing project when no project selected", () => {
    renderButton({ title: "do thing" });
    const button = screen.getByTestId("new-task-create-button") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    // The tooltip content is rendered as accessible-name on the wrapper.
    expect(screen.getByLabelText(/select a project to create a task/i)).toBeTruthy();
  });

  it("disables the button and labels missing title when title empty", () => {
    renderButton({ projectId: "proj-1" });
    const button = screen.getByTestId("new-task-create-button") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect(screen.getByLabelText(/add a title to create a task/i)).toBeTruthy();
  });

  it("enables the button without a tooltip when both title and project are set", () => {
    renderButton({ title: "do thing", projectId: "proj-1" });
    const button = screen.getByTestId("new-task-create-button") as HTMLButtonElement;
    expect(button.disabled).toBe(false);
    expect(screen.queryByLabelText(/create a task/i)).toBeNull();
  });
});
