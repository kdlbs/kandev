import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { TaskMR } from "@/lib/types/gitlab";

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
}));

import { MRRowTaskIndicator } from "./mr-row-task-indicator";

function renderIndicator(ui: ReactNode) {
  return render(
    <StateProvider>
      <TooltipProvider>{ui}</TooltipProvider>
    </StateProvider>,
  );
}

function makeAssociation(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "association-1",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 7,
    mr_url: "https://gitlab.com/group/project/-/merge_requests/7",
    mr_title: "Review this MR",
    head_branch: "feature",
    base_branch: "main",
    author_username: "alice",
    state: "opened",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

afterEach(cleanup);

describe("MRRowTaskIndicator", () => {
  it("shows an empty label before any task is linked", () => {
    renderIndicator(<MRRowTaskIndicator tasks={undefined} />);
    expect(screen.getByText("No task created yet")).toBeTruthy();
  });

  it("renders the linked task title for one association", () => {
    renderIndicator(<MRRowTaskIndicator tasks={[makeAssociation()]} />);
    expect(screen.getByRole("button").textContent).toContain("Review this MR");
  });

  it("renders a count for multiple linked tasks", () => {
    renderIndicator(
      <MRRowTaskIndicator
        tasks={[
          makeAssociation({ id: "one", task_id: "task-1" }),
          makeAssociation({ id: "two", task_id: "task-2" }),
        ]}
      />,
    );
    expect(screen.getByRole("button").textContent).toContain("Tasks");
    expect(screen.getByRole("button").textContent).toContain("2");
  });
});
