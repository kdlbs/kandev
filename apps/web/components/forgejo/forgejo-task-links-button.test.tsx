import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ForgejoTaskLinksButton } from "./forgejo-task-links-button";

const hooks = vi.hoisted(() => ({
  config: vi.fn(),
  links: vi.fn(),
  task: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: "ws-a" },
      tasks: { activeTaskId: "task-a" },
      repositories: {
        itemsByWorkspaceId: {
          "ws-a": [
            { id: "repo-a", provider_owner: "acme", provider_name: "app", default_branch: "main" },
          ],
        },
      },
    }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: hooks.toast }) }));
vi.mock("@/hooks/domains/forgejo/use-forgejo-config", () => ({ useForgejoConfig: hooks.config }));
vi.mock("@/hooks/domains/forgejo/use-forgejo-task-links", () => ({
  useForgejoTaskLinks: hooks.links,
}));
vi.mock("@/hooks/domains/kanban/use-task-by-id", () => ({ useTaskById: hooks.task }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({
  createForgejoTaskPullRequest: vi.fn(),
  linkForgejoIssue: vi.fn(),
  linkForgejoPullRequest: vi.fn(),
}));

describe("ForgejoTaskLinksButton", () => {
  beforeEach(() => {
    hooks.config.mockReturnValue({ config: { origin: "https://forgejo.example" } });
    hooks.links.mockReturnValue({
      issues: [],
      pullRequests: [],
      loading: false,
      error: null,
      reload: vi.fn(),
      refreshIssue: vi.fn(),
      refreshPullRequest: vi.fn(),
      removeIssue: vi.fn(),
      removePullRequest: vi.fn(),
    });
    hooks.task.mockReturnValue({
      id: "task-a",
      title: "Ship Forgejo",
      repositories: [
        {
          repository_id: "repo-a",
          base_branch: "main",
          checkout_branch: "feat/forgejo",
          position: 0,
        },
      ],
    });
  });

  it("does not expose Forgejo controls without a connected origin", () => {
    hooks.config.mockReturnValue({ config: null });
    render(<ForgejoTaskLinksButton />);
    expect(screen.queryByTestId("forgejo-task-links-button")).toBeNull();
  });

  it("prefills PR creation with the task worktree branch", async () => {
    render(<ForgejoTaskLinksButton />);
    const trigger = screen.getByTestId("forgejo-task-links-button");
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(await screen.findByText("Create pull request"));

    await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
    expect(screen.getByLabelText("Owner").getAttribute("value")).toBe("acme");
    expect(screen.getByLabelText("Repository").getAttribute("value")).toBe("app");
    expect(screen.getByLabelText("Source branch").getAttribute("value")).toBe("feat/forgejo");
    expect(screen.getByLabelText("Base branch").getAttribute("value")).toBe("main");
  });
});
