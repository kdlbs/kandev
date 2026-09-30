import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { RunError } from "@/app/office/tasks/[id]/types";
import { WebSocketRequestError } from "@/lib/ws/client";
import { RunErrorEntry } from "./run-error-entry";

const FAILURE_STAMP = "ordinary-failure-stamp";
const REMEDIATION_LINK_ID = "remediation-link";
const RECOVERY_ERROR_ID = "run-error-recovery-error";
const { requestMock, recoveryContext, appState } = vi.hoisted(() => ({
  requestMock: vi.fn(),
  recoveryContext: {
    value: null as { sessionId: string; model: ActiveSessionRecovery | null; pending: null } | null,
  },
  appState: {
    agentProfiles: { items: [] as unknown[] },
    kanban: { tasks: [] as unknown[] },
    quickChat: { sessions: [] as unknown[] },
    taskSessions: { items: {} as Record<string, unknown> },
  },
}));
const RUN_ERROR_RESUME_TEST_ID = "run-error-resume-button";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(appState),
}));
vi.mock("@/lib/state/slices/office/selectors", () => ({
  selectOfficeAgentProfiles: () => [],
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));
vi.mock("@/components/task/chat/session-recovery-context", () => ({
  useSessionComposerRecovery: () => recoveryContext.value,
}));

afterEach(() => {
  cleanup();
  recoveryContext.value = null;
  requestMock.mockReset().mockResolvedValue({});
  appState.agentProfiles.items = [];
  appState.kanban.tasks = [];
  appState.quickChat.sessions = [];
  appState.taskSessions.items = {};
});

function runError(failureCode: string): RunError {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentProfileId: "agent-1",
    rawPayload: "provider raw details",
    failedAt: "2026-08-20T10:00:00Z",
    failureCode,
    errorStamp: FAILURE_STAMP,
    message: "provider failure",
    remediationUrl: "https://opencode.ai/workspace/demo_workspace/go",
  };
}

it("keeps run remediation available when the composer has no recovery model", () => {
  recoveryContext.value = { sessionId: "session-1", model: null, pending: null };
  render(<RunErrorEntry taskId="task-1" error={runError("provider_auth_required")} />);
  expect(screen.getByTestId(REMEDIATION_LINK_ID)).toBeTruthy();
  expect(screen.getByTestId(RUN_ERROR_RESUME_TEST_ID)).toBeTruthy();
});

describe("RunErrorEntry", () => {
  it("renders managed npm policy failures on the runtime recovery surface", () => {
    render(
      <RunErrorEntry
        taskId="task-1"
        workspaceId="workspace-1"
        error={{
          ...runError("managed_runtime_npm_policy"),
          failureDetails:
            "npm error notarget No matching version found. Minimum release age policy applies.",
        }}
      />,
    );

    const recovery = screen.getByTestId("run-error-managed-runtime-npm-recovery");
    expect(recovery.textContent).toContain("npm blocked this runtime version");
    expect(recovery.textContent).toContain(
      "Check npm's min-release-age or before setting. Wait until this version is eligible or select an older version, then retry.",
    );
    expect(screen.getByTestId("run-error-managed-runtime-retry-button")).toBeTruthy();
    expect(screen.queryByTestId(RUN_ERROR_RESUME_TEST_ID)).toBeNull();
  });

  it.each(["provider_auth_required", "model_capacity"])(
    "keeps ordinary failure code %s on the resumable error surface",
    (failureCode) => {
      render(
        <RunErrorEntry taskId="task-1" workspaceId="workspace-1" error={runError(failureCode)} />,
      );

      expect(screen.getByTestId(RUN_ERROR_RESUME_TEST_ID)).toBeTruthy();

      expect(screen.getByTestId("run-error-fresh-button")).toBeTruthy();
      expect(
        screen.getByTestId("run-error-raw-payload").closest("details")?.hasAttribute("open"),
      ).toBe(false);
      expect(screen.getByTestId(REMEDIATION_LINK_ID)).toBeTruthy();
      expect(screen.queryByTestId("task-launch-error-entry")).toBeNull();
    },
  );

  it("retains a manual recovery error and exposes the typed branch action", async () => {
    requestMock.mockRejectedValueOnce(
      new WebSocketRequestError("The saved branch is no longer available.", "CONFLICT", {
        kind: "branch_unrecoverable",
        recovery_action: "resume_new_branch",
        original_branch: "feature/lost",
        base_branch: "main",
      }),
    );

    render(
      <RunErrorEntry
        taskId="task-1"
        workspaceId="workspace-1"
        error={runError("provider_auth_required")}
      />,
    );

    screen.getByTestId(RUN_ERROR_RESUME_TEST_ID).click();

    expect((await screen.findByTestId(RECOVERY_ERROR_ID)).textContent).toContain(
      "The saved branch is no longer available.",
    );
    expect(screen.getByTestId("run-error-continue-new-branch-button")).toBeTruthy();

    expect(screen.getByTestId("run-error-restore-workspace-button")).toBeTruthy();
  });

  it("renders recovered session history without stale recovery controls", () => {
    render(
      <RunErrorEntry
        taskId="task-1"
        workspaceId="workspace-1"
        error={{ ...runError("provider_auth_required"), isActive: false }}
      />,
    );

    expect(screen.queryByTestId(RUN_ERROR_RESUME_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("run-error-fresh-button")).toBeNull();
  });
});

describe("RunErrorEntry provider-restored Resume", () => {
  it("discloses skipped settings and sends provider-restored policy", async () => {
    appState.taskSessions.items = {
      "session-1": {
        task_id: "task-1",
        state: "FAILED",
        execution_profile_id: "profile-1",
        downstream_acp_session_id: "native-session-1",
      },
    };
    appState.kanban.tasks = [{ id: "task-1", isFromOffice: false }];
    appState.agentProfiles.items = [
      {
        id: "profile-1",
        agent_id: "opaque-agent-uuid",
        agent_name: "auggie",
        cli_passthrough: false,
      },
    ];

    render(
      <RunErrorEntry
        taskId="task-1"
        workspaceId="workspace-1"
        error={runError("provider_auth_required")}
      />,
    );

    const disclosure = screen.getByTestId("provider-restored-resume-disclosure");
    const resume = screen.getByTestId(RUN_ERROR_RESUME_TEST_ID);
    expect(
      disclosure.compareDocumentPosition(resume) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    fireEvent.click(resume);

    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith(
        "session.recover",
        {
          task_id: "task-1",
          session_id: "session-1",
          action: "resume",
          settings_policy: "provider_restored",
        },
        30_000,
      ),
    );
  });
});

it("explains a non-retryable recovery refusal without offering a bypass", async () => {
  requestMock.mockRejectedValueOnce(
    new WebSocketRequestError("blocked", "UNAVAILABLE", {
      kind: "session_recovery_unstoppable",
      retryable: false,
    }),
  );
  render(<RunErrorEntry taskId="task-1" error={runError("provider_auth_required")} />);
  fireEvent.click(screen.getByTestId(RUN_ERROR_RESUME_TEST_ID));
  const error = await screen.findByTestId(RECOVERY_ERROR_ID);
  expect(error.querySelector("p")?.textContent).toContain("Restart the backend");
  expect(screen.queryByTestId(RUN_ERROR_RESUME_TEST_ID)).toBeNull();
});

it("redacts credential-bearing branch failures in the status line", async () => {
  requestMock.mockRejectedValueOnce(
    new WebSocketRequestError("Branch failed: token=run-secret-fixture", "CONFLICT", {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
      original_branch: "feature/lost",
      base_branch: "main",
    }),
  );
  render(
    <RunErrorEntry
      taskId="task-1"
      workspaceId="workspace-1"
      error={runError("provider_auth_required")}
    />,
  );
  fireEvent.click(screen.getByTestId(RUN_ERROR_RESUME_TEST_ID));
  await screen.findByTestId(RECOVERY_ERROR_ID);
  expect(document.body.textContent).not.toContain("run-secret-fixture");
});

it("keeps an explanation when branch error sanitization removes all content", async () => {
  requestMock.mockRejectedValueOnce(
    new WebSocketRequestError("\u001b[31m\u001b[0m", "CONFLICT", {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
      original_branch: "feature/lost",
      base_branch: "main",
    }),
  );
  render(<RunErrorEntry taskId="task-1" error={runError("provider_auth_required")} />);
  fireEvent.click(screen.getByTestId(RUN_ERROR_RESUME_TEST_ID));
  const error = await screen.findByTestId(RECOVERY_ERROR_ID);
  expect(error.querySelector("p")?.textContent).toBe("Failed to resume session");
});

it("withholds workspace restore for a retryable run recovery guard", async () => {
  requestMock.mockRejectedValueOnce(
    new WebSocketRequestError("busy", "CONFLICT", {
      kind: "session_recovery_in_progress",
      retryable: true,
    }),
  );
  render(<RunErrorEntry taskId="task-1" error={runError("provider_auth_required")} />);
  fireEvent.click(screen.getByTestId(RUN_ERROR_RESUME_TEST_ID));
  await screen.findByTestId(RECOVERY_ERROR_ID);
  expect(screen.queryByTestId("run-error-restore-workspace-button")).toBeNull();
});

it("labels a workspace restoration failure separately from a resume failure", async () => {
  requestMock.mockRejectedValueOnce(new Error("Provider unavailable"));
  render(<RunErrorEntry taskId="task-1" error={runError("provider_auth_required")} />);
  fireEvent.click(screen.getByTestId(RUN_ERROR_RESUME_TEST_ID));
  const restore = await screen.findByTestId("run-error-restore-workspace-button");
  requestMock.mockRejectedValueOnce(new Error("\u001b[31m\u001b[0m"));
  fireEvent.click(restore);
  await screen.findByText("Failed to restore workspace");
  expect(screen.getByTestId(RECOVERY_ERROR_ID).querySelector("p")?.textContent).toBe(
    "Failed to restore workspace",
  );
});

it.each([
  { name: "a historical row", isActive: false, errorStamp: FAILURE_STAMP },
  { name: "a different failure", isActive: true, errorStamp: "another-stamp" },
  { name: "an unstamped failure", isActive: true, errorStamp: undefined },
])("preserves $name when another recovery card is present", ({ isActive, errorStamp }) => {
  recoveryContext.value = {
    sessionId: "session-1",
    model: { sessionId: "session-1", stamp: FAILURE_STAMP, kind: "generic" },
    pending: null,
  };
  render(
    <RunErrorEntry
      taskId="task-1"
      error={{ ...runError("provider_auth_required"), isActive, errorStamp }}
    />,
  );
  expect(screen.getByTestId("run-error-raw-payload").textContent).toBe("provider raw details");
  expect(screen.getByTestId(REMEDIATION_LINK_ID)).toBeTruthy();
  expect(Boolean(screen.queryByTestId(RUN_ERROR_RESUME_TEST_ID))).toBe(isActive);
});

it("compacts only the active failure owned by the composer card", () => {
  recoveryContext.value = {
    sessionId: "session-1",
    model: { sessionId: "session-1", stamp: FAILURE_STAMP, kind: "generic" },
    pending: null,
  };
  render(<RunErrorEntry taskId="task-1" error={runError("provider_auth_required")} />);
  expect(screen.queryByTestId(REMEDIATION_LINK_ID)).toBeNull();
  expect(screen.queryByTestId(RUN_ERROR_RESUME_TEST_ID)).toBeNull();
  expect(screen.getByText("provider raw details")).toBeTruthy();
});
