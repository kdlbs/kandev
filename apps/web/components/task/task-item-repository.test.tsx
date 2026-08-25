import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskItem } from "./task-item";

afterEach(() => cleanup());

const REPOSITORY_TEST_ID = "sidebar-task-repository";
const PRIMARY_REPOSITORY = "kdlbs/kandev";

function renderTaskItem(props: Partial<ComponentProps<typeof TaskItem>> = {}) {
  return render(
    <StateProvider>
      <TooltipProvider>
        <TaskItem title="Needs answer" state="REVIEW" {...props} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("TaskItem repository label", () => {
  it("names the repository the task belongs to", () => {
    renderTaskItem({ repositoryPath: PRIMARY_REPOSITORY });

    expect(screen.getByTestId(REPOSITORY_TEST_ID).textContent).toBe(PRIMARY_REPOSITORY);
  });

  it("keeps the full repository name reachable on hover when it is truncated", () => {
    renderTaskItem({ repositoryPath: "a-very-long-org-name/a-very-long-repository-name" });

    expect(screen.getByTestId(REPOSITORY_TEST_ID).getAttribute("title")).toBe(
      "a-very-long-org-name/a-very-long-repository-name",
    );
  });

  it("omits the repository when the list already groups by it", () => {
    renderTaskItem({ repositoryPath: PRIMARY_REPOSITORY, showRepository: false });

    expect(screen.queryByTestId(REPOSITORY_TEST_ID)).toBeNull();
  });

  it("renders nothing when the task has no repository", () => {
    renderTaskItem({});

    expect(screen.queryByTestId(REPOSITORY_TEST_ID)).toBeNull();
  });
});
