import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { taskPlanQueryOptions } from "@/lib/query/query-options";
import type { TaskPlan } from "@/lib/types/http";
import { LazyPlanPreview } from "./lazy-plan-preview";

vi.mock("@/lib/api/domains/plan-api", () => ({
  createTaskPlan: vi.fn(),
  deleteTaskPlan: vi.fn(),
  getPlanRevision: vi.fn(),
  getTaskPlan: vi.fn(),
  listPlanRevisions: vi.fn(),
  revertPlanRevision: vi.fn(),
  updateTaskPlan: vi.fn(),
}));

const TASK_ID = "task-1";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
}

function renderWithQuery(client: QueryClient, children: ReactNode) {
  return render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
}

function planFixture(content: string): TaskPlan {
  return {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content,
    created_by: "agent",
    created_at: "2026-06-23T00:00:00Z",
    updated_at: "2026-06-23T00:00:00Z",
  } as TaskPlan;
}

describe("LazyPlanPreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders cached Query plan content without a Zustand provider", () => {
    const client = createQueryClient();
    client.setQueryData<TaskPlan | null>(
      taskPlanQueryOptions(TASK_ID).queryKey,
      planFixture("## Plan\n\n1. Ship Query-backed context"),
    );

    renderWithQuery(client, <LazyPlanPreview taskId={TASK_ID} />);

    expect(screen.getByText("Plan")).toBeTruthy();
    expect(screen.getByText(/Ship Query-backed context/)).toBeTruthy();
  });
});
