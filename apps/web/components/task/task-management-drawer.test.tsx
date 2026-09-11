import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TaskManagementDrawer } from "./task-management-drawer";

afterEach(cleanup);
describe("task drawer navigation", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-004.3, AC-TASKS-THREADS-ACTIONS-004.6, AC-TASKS-THREADS-ACTIONS-001.3
  it("chooses a workflow step in one surface and backs through nested pages", () => {
    const onMove = vi.fn();
    render(
      <StateProvider>
        <TaskManagementDrawer
          task={{ id: "A", title: "Task A", workflowId: "one", workflowStepId: "s1" }}
          workflows={[
            { id: "one", name: "One" },
            { id: "two", name: "Two" },
          ]}
          stepsByWorkflowId={{
            one: [
              { id: "s1", title: "Initial" },
              { id: "s2", title: "Review" },
            ],
            two: [{ id: "s3", title: "Build" }],
          }}
          onMove={onMove}
          onPriority={vi.fn()}
          onArchive={vi.fn()}
          onDelete={vi.fn()}
          closeMenu={vi.fn()}
          linkActions={{}}
          onCloseAutoFocus={(event) => event.preventDefault()}
        />
      </StateProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Send to workflow" }));
    fireEvent.click(screen.getByRole("button", { name: "Two" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByRole("button", { name: "Two" })).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Two" }));
    fireEvent.click(screen.getByRole("button", { name: "Two" }));
    fireEvent.click(screen.getByRole("button", { name: "Build" }));
    expect(onMove).toHaveBeenCalledWith("two", "s3");
  });
});
