import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ForgejoPageClient } from "./forgejo-page-client";

const api = vi.hoisted(() => ({ createTask: vi.fn(), linkIssue: vi.fn() }));
const hooks = vi.hoisted(() => ({
  queue: vi.fn(),
  details: vi.fn(),
  appStore: vi.fn(),
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ createTask: api.createTask }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({ linkForgejoIssue: api.linkIssue }));
vi.mock("@/hooks/domains/forgejo/use-forgejo-queue", () => ({ useForgejoQueue: hooks.queue }));
vi.mock("@/hooks/domains/forgejo/use-forgejo-pull-request-details", () => ({
  useForgejoPullRequestDetails: hooks.details,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(hooks.appStore()),
}));

const issue = {
  number: 7,
  title: "Fix queue",
  html_url: "https://forgejo.example/acme/app/issues/7",
  body: "Details",
  state: "open",
};
const appState = {
  workflows: { activeId: "workflow-a" },
  kanbanMulti: { snapshots: { "workflow-a": { steps: [{ id: "step-a", is_start_step: true }] } } },
};

describe("ForgejoPageClient", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    hooks.queue.mockReturnValue({
      queue: {
        issues: [{ repository: { owner: "acme", name: "app", full_name: "acme/app" }, issue }],
        pull_requests: [],
      },
      loading: false,
      error: null,
      refresh: vi.fn(),
    });
    hooks.details.mockReturnValue({
      details: null,
      loading: false,
      error: null,
      load: vi.fn(),
      comment: vi.fn(),
      review: vi.fn(),
    });
    hooks.appStore.mockReturnValue(appState);
    api.createTask.mockResolvedValue({ id: "task-a", title: issue.title });
    api.linkIssue.mockResolvedValue({ id: "link-a" });
  });

  it("creates and links a Kandev task from a queued Forgejo issue", async () => {
    render(<ForgejoPageClient workspaceId="ws-a" />);
    fireEvent.click(screen.getByRole("button", { name: "Create Kandev task" }));
    await waitFor(() =>
      expect(api.createTask).toHaveBeenCalledWith(
        expect.objectContaining({
          workspace_id: "ws-a",
          workflow_id: "workflow-a",
          workflow_step_id: "step-a",
          title: "Fix queue",
        }),
      ),
    );
    expect(api.linkIssue).toHaveBeenCalledWith(
      { task_id: "task-a", owner: "acme", repo: "app", number: 7 },
      { workspaceId: "ws-a" },
    );
    expect(await screen.findByText(/Created Kandev task/)).toBeTruthy();
  });
});
