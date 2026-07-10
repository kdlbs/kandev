import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
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

const INPUT_TEST_ID = "task-external-link-input";
const SUBMIT_TEST_ID = "task-external-link-submit";
const ERROR_TEST_ID = "task-external-link-error";
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";
const TASK_TITLE = "Fix login";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderDialog(provider: "jira" | "linear" | "sentry" = "jira") {
  return render(
    <StateProvider>
      <ToastProvider>
        <TaskExternalLinkDialog
          open={true}
          onOpenChange={vi.fn()}
          provider={provider}
          task={{ id: TASK_ID, title: TASK_TITLE }}
          workspaceId={WORKSPACE_ID}
        />
      </ToastProvider>
    </StateProvider>,
  );
}

describe("TaskExternalLinkDialog", () => {
  it("validates and links a Jira ticket by updating the task title", async () => {
    vi.mocked(getJiraTicket).mockResolvedValue({ key: "PROJ-12" } as never);
    vi.mocked(updateTask).mockResolvedValue({ id: TASK_ID, title: "PROJ-12: Fix login" } as never);

    renderDialog("jira");

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), {
      target: { value: "PROJ-12" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    await waitFor(() => {
      expect(getJiraTicket).toHaveBeenCalledWith("PROJ-12", { workspaceId: WORKSPACE_ID });
    });
    expect(updateTask).toHaveBeenCalledWith(TASK_ID, { title: "PROJ-12: Fix login" });
  });

  it("validates and links a Linear issue by updating the task title", async () => {
    vi.mocked(getLinearIssue).mockResolvedValue({ identifier: "ENG-20" } as never);
    vi.mocked(updateTask).mockResolvedValue({ id: TASK_ID, title: "ENG-20: Fix login" } as never);

    renderDialog("linear");

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), {
      target: { value: "eng-20" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    await waitFor(() => {
      expect(getLinearIssue).toHaveBeenCalledWith("ENG-20", { workspaceId: WORKSPACE_ID });
    });
    expect(updateTask).toHaveBeenCalledWith(TASK_ID, { title: "ENG-20: Fix login" });
  });

  it("extracts and links a Sentry issue key from a numeric issue URL", async () => {
    vi.mocked(getSentryIssue).mockResolvedValue({ shortId: "API-42" } as never);
    vi.mocked(updateTask).mockResolvedValue({ id: TASK_ID, title: "API-42: Fix login" } as never);

    renderDialog("sentry");

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), {
      target: { value: "https://sentry.io/organizations/acme/issues/123456/" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    await waitFor(() => {
      expect(getSentryIssue).toHaveBeenCalledWith("123456", { workspaceId: WORKSPACE_ID });
    });
    expect(updateTask).toHaveBeenCalledWith(TASK_ID, { title: "API-42: Fix login" });
  });

  it("shows an inline validation error for invalid input", async () => {
    renderDialog("jira");

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), {
      target: { value: "not a key" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    expect((await screen.findByTestId(ERROR_TEST_ID)).textContent).toMatch(
      /Paste a Jira ticket URL or key/i,
    );
    expect(updateTask).not.toHaveBeenCalled();

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), {
      target: { value: "PROJ-12" },
    });

    expect(screen.queryByTestId(ERROR_TEST_ID)).toBeNull();
  });
});
