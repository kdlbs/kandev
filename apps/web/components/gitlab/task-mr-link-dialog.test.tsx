import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Repository } from "@/lib/types/http";
import { TaskMRLinkDialog } from "./task-mr-link-dialog";

const appState = { setTaskMR: vi.fn() };
const createTaskMR = vi.hoisted(() => vi.fn().mockResolvedValue({}));
const repositories = [
  {
    id: "repository-1",
    name: "kandev",
    provider_owner: "platform",
    provider_name: "kandev",
  },
  {
    id: "repository-2",
    name: "docs",
    provider_owner: "platform",
    provider_name: "docs",
  },
] as Repository[];
const taskRepositories = repositories.map((repository) => ({ repository_id: repository.id }));
const mrURL = "https://gitlab.example.test/platform/kandev/-/merge_requests/81";
const mergeRequestURLLabel = "Merge request URL";
const linkMergeRequestButton = "Link merge request";
const workspaceId = "workspace-1";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appState) => unknown) => selector(appState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => ({ createTaskMR }));

function dialog({
  open = true,
  repositoryOptions = repositories,
  taskRepositoryLinks = taskRepositories,
}: {
  open?: boolean;
  repositoryOptions?: Repository[];
  taskRepositoryLinks?: { repository_id: string }[];
} = {}) {
  return (
    <TaskMRLinkDialog
      open={open}
      onOpenChange={vi.fn()}
      taskId="task-1"
      workspaceId={workspaceId}
      taskRepositories={taskRepositoryLinks}
      repositories={repositoryOptions}
    />
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("TaskMRLinkDialog", () => {
  it("submits the default repository when initially mounted open", async () => {
    render(dialog());

    fireEvent.change(screen.getByLabelText(mergeRequestURLLabel), {
      target: { value: mrURL },
    });
    fireEvent.click(screen.getByRole("button", { name: linkMergeRequestButton }));

    await waitFor(() => expect(createTaskMR).toHaveBeenCalledTimes(1));
    expect(createTaskMR).toHaveBeenCalledWith(
      {
        task_id: "task-1",
        repository_id: "repository-1",
        mr_url: mrURL,
      },
      workspaceId,
    );
  });

  it("uses the default repository when repositories hydrate after opening", async () => {
    const { rerender } = render(dialog({ repositoryOptions: [], taskRepositoryLinks: [] }));

    rerender(dialog());
    fireEvent.change(screen.getByLabelText(mergeRequestURLLabel), {
      target: { value: mrURL },
    });
    fireEvent.click(screen.getByRole("button", { name: linkMergeRequestButton }));

    await waitFor(() => expect(createTaskMR).toHaveBeenCalledTimes(1));
    expect(createTaskMR).toHaveBeenCalledWith(
      expect.objectContaining({ repository_id: "repository-1" }),
      workspaceId,
    );
  });

  it("preserves the typed URL and repository selection when props refresh while open", async () => {
    const { rerender } = render(dialog());

    const url = screen.getByLabelText(mergeRequestURLLabel);
    fireEvent.change(url, {
      target: { value: mrURL },
    });
    fireEvent.click(screen.getByRole("combobox", { name: "Task repository" }));
    fireEvent.click(await screen.findByRole("option", { name: "platform/docs" }));

    const newlyHydratedRepository = {
      id: "repository-3",
      name: "api",
      provider_owner: "platform",
      provider_name: "api",
    } as Repository;
    rerender(
      dialog({
        repositoryOptions: [newlyHydratedRepository, ...repositories],
        taskRepositoryLinks: [{ repository_id: newlyHydratedRepository.id }, ...taskRepositories],
      }),
    );

    expect((url as HTMLInputElement).value).toBe(mrURL);
    fireEvent.click(screen.getByRole("button", { name: linkMergeRequestButton }));

    await waitFor(() => expect(createTaskMR).toHaveBeenCalledTimes(1));
    expect(createTaskMR).toHaveBeenCalledWith(
      expect.objectContaining({
        repository_id: "repository-2",
        mr_url: mrURL,
      }),
      workspaceId,
    );
  });

  it("resets the URL and selection to the current default after closing", async () => {
    const { rerender } = render(dialog());

    fireEvent.change(screen.getByLabelText(mergeRequestURLLabel), {
      target: { value: mrURL },
    });
    const reorderedLinks = [taskRepositories[1], taskRepositories[0]];
    rerender(dialog({ open: false, taskRepositoryLinks: reorderedLinks }));
    rerender(dialog({ taskRepositoryLinks: reorderedLinks }));

    const url = screen.getByLabelText(mergeRequestURLLabel) as HTMLInputElement;
    expect(url.value).toBe("");
    fireEvent.change(url, {
      target: { value: "https://gitlab.example.test/platform/docs/-/merge_requests/12" },
    });
    fireEvent.click(screen.getByRole("button", { name: linkMergeRequestButton }));

    await waitFor(() => expect(createTaskMR).toHaveBeenCalledTimes(1));
    expect(createTaskMR).toHaveBeenCalledWith(
      expect.objectContaining({ repository_id: "repository-2" }),
      workspaceId,
    );
  });
});
