import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import {
  repositoryId,
  taskId,
  workflowId,
  workspaceId,
  type Repository,
  type Task,
} from "@/lib/types/http";

type MockState = {
  kanban: { tasks: [] };
  environmentIdBySessionId: Record<string, string | undefined>;
  bumpSessionCommitsRefetch: ReturnType<typeof vi.fn>;
};

const TIMESTAMP = "2026-01-01T00:00:00Z";

const repositories: Repository[] = [
  {
    id: repositoryId("repo-1"),
    workspace_id: workspaceId("ws-1"),
    name: "repo-one",
    source_type: "local",
    local_path: "/repo-one",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  },
];

let mockState: MockState;

vi.mock("@tabler/icons-react", () => ({
  IconChevronDown: () => <span />,
  IconLoader2: () => <span />,
}));

vi.mock("@kandev/ui/popover", () => ({
  Popover: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  PopoverContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  PopoverTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/workspace/use-repository-cache", () => ({
  useAllCachedRepositories: () => repositories,
}));

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({ branches: [], isLoading: false }),
}));

vi.mock("@/hooks/use-environment-session-id", () => ({
  useEnvironmentSessionId: () => "session-1",
}));

vi.mock("@/hooks/domains/session/use-cumulative-diff", () => ({
  invalidateCumulativeDiffCache: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
  updateTaskRepositoryBaseBranch: vi.fn(),
}));

import { BaseBranchPicker } from "./base-branch-picker";

function makeTask(): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-1",
    position: 1,
    title: "Query task",
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [
      {
        id: "task-repo-1",
        task_id: taskId("task-1"),
        repository_id: repositoryId("repo-1"),
        base_branch: "main",
        position: 0,
        created_at: TIMESTAMP,
        updated_at: TIMESTAMP,
      },
    ],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("BaseBranchPicker", () => {
  beforeEach(() => {
    mockState = {
      kanban: { tasks: [] },
      environmentIdBySessionId: { "session-1": "env-1" },
      bumpSessionCommitsRefetch: vi.fn(),
    };
  });

  it("resolves the task repository link from task detail Query when kanban tasks are empty", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());

    render(<BaseBranchPicker taskId="task-1" repositoryName="" fallbackBaseBranch="fallback" />, {
      wrapper: wrapper(client),
    });

    expect(screen.getByTestId("base-branch-picker-trigger").textContent).toContain("main");
  });
});
