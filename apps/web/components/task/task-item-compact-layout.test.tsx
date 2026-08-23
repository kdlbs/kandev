import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskItem } from "./task-item";

afterEach(() => cleanup());

describe("TaskItem compact layout", () => {
  it("centers the title content when the details row is disabled", () => {
    render(
      <StateProvider>
        <TooltipProvider>
          <TaskItem
            title="Needs answer"
            state="REVIEW"
            taskRowPresentation={{
              detailsEnabled: false,
              detailOrder: ["relative_time", "repository", "pull_request_number"],
              visibleDetails: [],
              trailing: "none",
            }}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    const row = screen.getByTestId("sidebar-task-item");
    expect(row.className).toContain("items-center");
    expect(row.className).not.toContain("items-start");
  });
});
