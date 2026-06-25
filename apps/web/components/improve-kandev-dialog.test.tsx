import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import { ImproveKandevDialog } from "./improve-kandev-dialog";

const mocks = vi.hoisted(() => ({
  bootstrapImproveKandev: vi.fn(),
  fetchSystemHealth: vi.fn(),
  listRepositories: vi.fn(),
  listWorkflowSteps: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/components/improve-kandev-dialog-create", () => ({
  CreateModeView: ({ bootstrap }: { bootstrap: { kind: string; steps?: unknown[] } }) => (
    <div
      data-testid="create-mode"
      data-bootstrap-kind={bootstrap.kind}
      data-step-count={bootstrap.steps?.length ?? 0}
    />
  ),
}));

vi.mock("@/lib/api/domains/health-api", () => ({
  fetchSystemHealth: (...args: unknown[]) => mocks.fetchSystemHealth(...args),
}));

vi.mock("@/lib/api/domains/improve-kandev-api", () => ({
  bootstrapImproveKandev: (...args: unknown[]) => mocks.bootstrapImproveKandev(...args),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: (...args: unknown[]) => mocks.listWorkflowSteps(...args),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listBranches: vi.fn(),
  listQuickChatSessions: vi.fn(),
  listRepositories: (...args: unknown[]) => mocks.listRepositories(...args),
  listRepositoryBranches: vi.fn(),
  listRepositoryScripts: vi.fn(),
  listWorkspaces: vi.fn(),
}));

function renderDialog(queryClient: QueryClient) {
  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider>
        <ImproveKandevDialog open onOpenChange={() => {}} workspaceId="ws-1" />
      </StateProvider>
    </QueryClientProvider>,
  );
}

describe("ImproveKandevDialog", () => {
  beforeEach(() => {
    mocks.bootstrapImproveKandev.mockResolvedValue({
      repository_id: "repo-1",
      workflow_id: "wf-1",
      branch: "improve/example",
      bundle_dir: "/tmp/bundle",
      bundle_files: {
        metadata: "metadata.json",
        backend_log: "backend.log",
        frontend_log: "frontend.log",
      },
      github_login: "octo",
      has_write_access: false,
      fork_status: "ready",
    });
    mocks.fetchSystemHealth.mockResolvedValue({ issues: [] });
    mocks.listRepositories.mockRejectedValue(new Error("raw repository fetch should not run"));
    mocks.listWorkflowSteps.mockRejectedValue(new Error("raw step fetch should not run"));
    mocks.toast.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("hydrates bootstrap steps and repositories through Query cache", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const steps = [
      {
        id: "step-1",
        workflow_id: "wf-1",
        name: "Improve",
        position: 0,
        color: "bg-blue-500",
        is_start_step: true,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ];
    const repositories = [{ id: "repo-link-1", repository_id: "repo-1", name: "kandev" }];
    mocks.listWorkflowSteps.mockResolvedValue({ steps });
    mocks.listRepositories.mockResolvedValue({ repositories });

    renderDialog(queryClient);

    const proceed = await screen.findByTestId("improve-kandev-proceed");
    await waitFor(() => expect((proceed as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(proceed);

    await waitFor(() =>
      expect(screen.getByTestId("create-mode").getAttribute("data-bootstrap-kind")).toBe("ready"),
    );
    expect(screen.getByTestId("create-mode").getAttribute("data-step-count")).toBe("1");
    expect(queryClient.getQueryData(qk.workflows.steps("wf-1"))).toEqual(steps);
    expect(queryClient.getQueryData(qk.workspaces.repositories("ws-1"))).toEqual(repositories);
    expect(mocks.toast).not.toHaveBeenCalled();
  });
});
