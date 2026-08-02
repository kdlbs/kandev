import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AzureDevOpsPullRequest, AzureDevOpsWorkItem } from "@/lib/types/azure-devops";
import { AzureDevOpsPullRequestResults, AzureDevOpsWorkItemResults } from "./azure-devops-results";

afterEach(cleanup);

describe("Azure DevOps results", () => {
  it("shows a refresh error instead of stale work-item actions", () => {
    render(
      <AzureDevOpsWorkItemResults
        items={[{ id: 1, title: "Stale" } as AzureDevOpsWorkItem]}
        loading={false}
        error="Refresh failed"
        onStartTask={vi.fn()}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain("Refresh failed");
    expect(screen.queryByRole("button", { name: "Start task" })).toBeNull();
  });

  it("shows loading instead of stale pull-request actions", () => {
    render(
      <AzureDevOpsPullRequestResults
        items={[{ id: 42, title: "Stale" } as AzureDevOpsPullRequest]}
        loading
        error={null}
        onFeedback={vi.fn()}
        onStartTask={vi.fn()}
      />,
    );
    expect(screen.getByText("Loading results...")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Feedback" })).toBeNull();
  });

  it("offers configured work-item quick actions before creating a task", () => {
    const onQuickAction = vi.fn();
    render(
      <AzureDevOpsWorkItemResults
        items={[{ id: 7, title: "Fix Azure" } as AzureDevOpsWorkItem]}
        loading={false}
        error={null}
        onStartTask={vi.fn()}
        quickActions={[
          {
            id: "implement",
            label: "Implement",
            hint: "Build it",
            icon: "code",
            promptTemplate: "Implement {{url}}",
          },
        ]}
        onQuickAction={onQuickAction}
      />,
    );

    fireEvent.pointerDown(screen.getByRole("button", { name: "Task actions for work item 7" }));
    fireEvent.click(screen.getByRole("menuitem", { name: /Implement/ }));
    expect(onQuickAction).toHaveBeenCalledWith(
      expect.objectContaining({ id: 7 }),
      expect.anything(),
    );
  });
});
