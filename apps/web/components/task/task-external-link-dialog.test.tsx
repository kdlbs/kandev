import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { useSentryInstances } from "@/hooks/domains/sentry/use-sentry-availability";
import type { SentryAvailability } from "@/hooks/domains/sentry/use-sentry-availability";
import type { SentryConfig } from "@/lib/types/sentry";
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

vi.mock("@/hooks/domains/sentry/use-sentry-availability", () => ({
  useSentryInstances: vi.fn(),
  isHealthySentryInstance: (i: SentryConfig) => i.hasSecret && i.lastOk,
}));

const INPUT_TEST_ID = "task-external-link-input";
const SUBMIT_TEST_ID = "task-external-link-submit";
const ERROR_TEST_ID = "task-external-link-error";
const INSTANCE_SELECT_TEST_ID = "sentry-link-instance-select";
const NO_INSTANCE_TEST_ID = "sentry-link-no-instance";
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";
const TASK_TITLE = "Fix login";

function instance(id: string, name: string): SentryConfig {
  return {
    id,
    workspaceId: WORKSPACE_ID,
    name,
    authMethod: "auth_token",
    url: "https://sentry.io",
    hasSecret: true,
    lastOk: true,
    lastError: "",
    createdAt: "",
    updatedAt: "",
  };
}

function mockSentryInstances(state: SentryAvailability["state"], healthy: SentryConfig[]) {
  vi.mocked(useSentryInstances).mockReturnValue({
    loading: false,
    instances: healthy,
    healthy,
    available: healthy.length > 0,
    state,
  });
}

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

describe("TaskExternalLinkDialog — Jira and Linear", () => {
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
});

describe("TaskExternalLinkDialog — Sentry linking", () => {
  it("auto-selects the sole healthy Sentry instance and links against it", async () => {
    mockSentryInstances("single", [instance("inst-1", "Production")]);
    vi.mocked(getSentryIssue).mockResolvedValue({ shortId: "API-42" } as never);
    vi.mocked(updateTask).mockResolvedValue({ id: TASK_ID, title: "API-42: Fix login" } as never);

    renderDialog("sentry");

    // The sole instance auto-selects, so no picker is shown.
    expect(screen.queryByTestId(INSTANCE_SELECT_TEST_ID)).toBeNull();

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), {
      target: { value: "https://sentry.io/organizations/acme/issues/123456/" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    // The chosen instance id is threaded into the strict browse contract.
    await waitFor(() => {
      expect(getSentryIssue).toHaveBeenCalledWith(WORKSPACE_ID, "inst-1", "123456");
    });
    expect(updateTask).toHaveBeenCalledWith(TASK_ID, { title: "API-42: Fix login" });
  });

  it("prompts for a Sentry instance when several are healthy and blocks submit", () => {
    mockSentryInstances("multi", [
      instance("inst-1", "Production"),
      instance("inst-2", "Self-hosted"),
    ]);

    renderDialog("sentry");

    // A picker is shown and submit stays disabled until an instance is chosen.
    expect(screen.getByTestId(INSTANCE_SELECT_TEST_ID)).toBeTruthy();
    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), { target: { value: "PROJ-1" } });
    const submit = screen.getByTestId(SUBMIT_TEST_ID) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.click(submit);
    expect(getSentryIssue).not.toHaveBeenCalled();
  });

  it("explains and blocks linking when the workspace has no healthy Sentry instance", () => {
    mockSentryInstances("empty", []);

    renderDialog("sentry");

    expect(screen.getByTestId(NO_INSTANCE_TEST_ID)).toBeTruthy();
    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), { target: { value: "PROJ-1" } });
    const submit = screen.getByTestId(SUBMIT_TEST_ID) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.click(submit);
    expect(getSentryIssue).not.toHaveBeenCalled();
  });
});

describe("TaskExternalLinkDialog — validation", () => {
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

describe("TaskExternalLinkDialog — Sentry availability", () => {
  it("clears a selected Sentry instance when it is no longer healthy", async () => {
    mockSentryInstances("multi", [
      instance("inst-1", "Production"),
      instance("inst-2", "Self-hosted"),
    ]);
    renderDialog("sentry");

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), { target: { value: "PROJ-1" } });
    fireEvent.click(screen.getByTestId(INSTANCE_SELECT_TEST_ID));
    fireEvent.click(await screen.findByRole("option", { name: "Production" }));
    await waitFor(() => {
      expect((screen.getByTestId(SUBMIT_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
    });

    mockSentryInstances("multi", [
      instance("inst-2", "Self-hosted"),
      instance("inst-3", "Staging"),
    ]);
    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), { target: { value: "PROJ-2" } });

    await waitFor(() => {
      expect((screen.getByTestId(SUBMIT_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    });
  });
});

describe("TaskExternalLinkDialog — Sentry sole-instance swap", () => {
  it("retargets the link when a poll swaps the sole healthy instance (state stays single)", async () => {
    mockSentryInstances("single", [instance("inst-1", "Alpha")]);
    vi.mocked(getSentryIssue).mockResolvedValue({ shortId: "PROJ-2" } as never);
    vi.mocked(updateTask).mockResolvedValue({ id: TASK_ID, title: "PROJ-2: Fix login" } as never);

    const view = renderDialog("sentry");
    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), { target: { value: "PROJ-1" } });
    await waitFor(() => {
      expect((screen.getByTestId(SUBMIT_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
    });

    // A polling refresh replaces the sole healthy instance (inst-1 -> inst-2)
    // while availability state remains "single". The effect must retarget the
    // new instance rather than link against the stale inst-1.
    mockSentryInstances("single", [instance("inst-2", "Beta")]);
    view.rerender(
      <StateProvider>
        <ToastProvider>
          <TaskExternalLinkDialog
            open={true}
            onOpenChange={vi.fn()}
            provider="sentry"
            task={{ id: TASK_ID, title: TASK_TITLE }}
            workspaceId={WORKSPACE_ID}
          />
        </ToastProvider>
      </StateProvider>,
    );

    fireEvent.change(screen.getByTestId(INPUT_TEST_ID), { target: { value: "PROJ-2" } });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    await waitFor(() => {
      expect(getSentryIssue).toHaveBeenCalledWith(WORKSPACE_ID, "inst-2", "PROJ-2");
    });
  });
});
