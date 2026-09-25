import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RunError } from "@/app/office/tasks/[id]/types";
import { WebSocketRequestError } from "@/lib/ws/client";
import { RunErrorEntry } from "./run-error-entry";

const { requestMock } = vi.hoisted(() => ({ requestMock: vi.fn() }));
const RUN_ERROR_RESUME_TEST_ID = "run-error-resume-button";

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
