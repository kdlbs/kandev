import { cleanup, render, screen } from "@testing-library/react";
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
});
