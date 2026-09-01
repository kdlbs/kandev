import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RunError } from "@/app/office/tasks/[id]/types";
import { WebSocketRequestError } from "@/lib/ws/client";
import { RunErrorEntry } from "./run-error-entry";

const { requestMock } = vi.hoisted(() => ({ requestMock: vi.fn() }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector({}),
}));
vi.mock("@/lib/state/slices/office/selectors", () => ({
  selectOfficeAgentProfiles: () => [],
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

afterEach(() => cleanup());

function runError(failureCode: string): RunError {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentProfileId: "agent-1",
    rawPayload: "provider raw details",
    failedAt: "2026-08-20T10:00:00Z",
    failureCode,
    errorStamp: "ordinary-failure-stamp",
    message: "provider failure",
    remediationUrl: "https://opencode.ai/workspace/demo_workspace/go",
  };
}

describe("RunErrorEntry", () => {
  it.each(["provider_auth_required", "model_capacity"])(
    "keeps ordinary failure code %s on the resumable error surface",
    (failureCode) => {
      render(
        <RunErrorEntry taskId="task-1" workspaceId="workspace-1" error={runError(failureCode)} />,
      );

      expect(screen.getByTestId("run-error-resume-button")).toBeTruthy();
      expect(screen.getByTestId("run-error-fresh-button")).toBeTruthy();
      expect(screen.queryByTestId("run-error-raw-payload")).toBeNull();
      expect(screen.getByTestId("remediation-link")).toBeTruthy();
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

    screen.getByTestId("run-error-resume-button").click();

    expect(await screen.findByText("The saved branch is no longer available.")).toBeTruthy();
    expect(screen.getByTestId("run-error-continue-new-branch-button")).toBeTruthy();
    expect(screen.getByTestId("run-error-restore-workspace-button")).toBeTruthy();
  });
});
