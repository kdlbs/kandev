import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Repository } from "@/lib/types/http";
import { TaskMRLinkDialog } from "./task-mr-link-dialog";

const appState = { setTaskMR: vi.fn() };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appState) => unknown) => selector(appState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  createTaskMR: vi.fn(),
}));

afterEach(cleanup);

describe("TaskMRLinkDialog", () => {
  it("preserves the typed URL when repository props refresh while open", () => {
    const repository = {
      id: "repository-1",
      name: "kandev",
      provider_owner: "platform",
      provider_name: "kandev",
    } as Repository;
    const props = {
      open: true,
      onOpenChange: vi.fn(),
      taskId: "task-1",
      workspaceId: "workspace-1",
    };
    const { rerender } = render(
      <TaskMRLinkDialog
        {...props}
        taskRepositories={[{ repository_id: repository.id }]}
        repositories={[repository]}
      />,
    );

    const url = screen.getByLabelText("Merge request URL");
    fireEvent.change(url, {
      target: { value: "https://gitlab.example.test/platform/kandev/-/merge_requests/81" },
    });

    rerender(
      <TaskMRLinkDialog
        {...props}
        taskRepositories={[{ repository_id: repository.id }]}
        repositories={[{ ...repository }]}
      />,
    );

    expect((url as HTMLInputElement).value).toBe(
      "https://gitlab.example.test/platform/kandev/-/merge_requests/81",
    );
    expect(
      (screen.getByRole("button", { name: "Link merge request" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });
});
