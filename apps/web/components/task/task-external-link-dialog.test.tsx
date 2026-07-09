import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { TaskExternalLinkDialog } from "./task-external-link-dialog";

vi.mock("@/lib/api/domains/jira-api", () => ({
  getJiraTicket: vi.fn(),
}));

vi.mock("@/lib/api/domains/linear-api", () => ({
  getLinearIssue: vi.fn(),
}));

vi.mock("@/lib/api/domains/sentry-api", () => ({
  getSentryIssue: vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  updateTask: vi.fn(),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderDialog(provider: "jira" | "linear" | "sentry" = "jira") {
  return render(
    <ToastProvider>
      <TaskExternalLinkDialog
        open={true}
        onOpenChange={vi.fn()}
        provider={provider}
        task={{ id: "task-1", title: "Fix login" }}
        workspaceId="workspace-1"
      />
    </ToastProvider>,
  );
}

describe("TaskExternalLinkDialog", () => {
  it("validates and links a Jira ticket by updating the task title", async () => {
    vi.mocked(getJiraTicket).mockResolvedValue({ key: "PROJ-12" } as never);
    vi.mocked(updateTask).mockResolvedValue({ id: "task-1", title: "PROJ-12: Fix login" } as never);

    renderDialog("jira");

    fireEvent.change(screen.getByTestId("task-external-link-input"), {
      target: { value: "PROJ-12" },
    });
    fireEvent.click(screen.getByTestId("task-external-link-submit"));

    await waitFor(() => {
      expect(getJiraTicket).toHaveBeenCalledWith("PROJ-12", { workspaceId: "workspace-1" });
    });
    expect(updateTask).toHaveBeenCalledWith("task-1", { title: "PROJ-12: Fix login" });
  });

  it("shows an inline validation error for invalid input", async () => {
    renderDialog("jira");

    fireEvent.change(screen.getByTestId("task-external-link-input"), {
      target: { value: "not a key" },
    });
    fireEvent.click(screen.getByTestId("task-external-link-submit"));

    expect((await screen.findByTestId("task-external-link-error")).textContent).toMatch(
      /Paste a Jira ticket URL or key/i,
    );
    expect(updateTask).not.toHaveBeenCalled();
  });
});
