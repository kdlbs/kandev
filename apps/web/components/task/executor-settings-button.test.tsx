import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";

let mockPrepareState: SessionPrepareState | null = null;
let mockEnv: { executor_type: string; sandbox_id?: string; container_id?: string } | null = null;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      prepareProgress: {
        bySessionId: mockPrepareState ? { [mockPrepareState.sessionId]: mockPrepareState } : {},
      },
    }),
}));

vi.mock("@/lib/api/domains/task-environment-api", () => ({
  fetchTaskEnvironmentLive: vi.fn().mockImplementation(async () => ({
    environment: mockEnv ?? { executor_type: "" },
    container: null,
  })),
  resetTaskEnvironment: vi.fn().mockResolvedValue({ success: true }),
}));

vi.mock("./task-reset-env-confirm-dialog", () => ({
  TaskResetEnvConfirmDialog: () => null,
}));

import { ExecutorSettingsButton } from "./executor-settings-button";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const STEP_CREATE_SANDBOX = "Creating cloud sandbox";

describe("ExecutorSettingsButton", () => {
  afterEach(() => {
    cleanup();
    mockPrepareState = null;
    mockEnv = null;
  });

  it("shows the cloud icon when the executor type is sprites", async () => {
    mockEnv = { executor_type: "sprites", sandbox_id: "kandev-abc" };

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);

    // Wait a tick for the live fetch to resolve.
    await Promise.resolve();
    await Promise.resolve();
    expect(await screen.findByTestId("executor-status-cloud-icon")).toBeTruthy();
  });

  it("shows the container icon for both docker variants", async () => {
    mockEnv = { executor_type: "local_docker", container_id: "abcdef" };

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);

    await Promise.resolve();
    expect(await screen.findByTestId("executor-status-container-icon")).toBeTruthy();
  });

  it("swaps to a spinner while the prepare progress is preparing", async () => {
    mockEnv = { executor_type: "sprites", sandbox_id: "kandev-abc" };
    mockPrepareState = {
      sessionId: SESSION_ID,
      status: "preparing",
      steps: [{ name: STEP_CREATE_SANDBOX, status: "running" }],
    };

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);

    expect(screen.getByTestId("executor-settings-button-spinner")).toBeTruthy();
    expect(screen.queryByTestId("executor-status-cloud-icon")).toBeNull();
  });

  it("renders the preparing section with current step copy", async () => {
    mockPrepareState = {
      sessionId: SESSION_ID,
      status: "preparing",
      steps: [
        { name: STEP_CREATE_SANDBOX, status: "completed" },
        { name: "Uploading agent controller", status: "running" },
        { name: "Waiting for agent controller", status: "pending" },
      ],
    };

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);

    // Open the popover by clicking the trigger.
    const trigger = screen.getByTestId("executor-settings-button");
    trigger.click();

    expect(await screen.findByTestId("executor-prepare-status")).toHaveProperty(
      "dataset.phase",
      "preparing",
    );
    expect(screen.getByText(/Step 2 of 3: Uploading agent controller/)).toBeTruthy();
  });

  it("renders the fallback warning callout when the missing-sandbox notice is present", async () => {
    mockPrepareState = {
      sessionId: SESSION_ID,
      status: "preparing",
      steps: [
        {
          name: "Reconnecting cloud sandbox",
          status: "skipped",
          warning:
            "Previous sandbox is no longer available — provisioning a fresh one for this branch.",
        },
        { name: STEP_CREATE_SANDBOX, status: "running" },
      ],
    };

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);
    screen.getByTestId("executor-settings-button").click();

    const status = await screen.findByTestId("executor-prepare-status");
    expect(status.dataset.phase).toBe("preparing_fallback");
    expect(screen.getByTestId("executor-prepare-fallback-warning")).toBeTruthy();
  });

  it("shows an Environment ready row once preparation completes", async () => {
    mockPrepareState = {
      sessionId: SESSION_ID,
      status: "completed",
      steps: [{ name: "Creating cloud sandbox", status: "completed" }],
      durationMs: 12500,
    };

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);
    screen.getByTestId("executor-settings-button").click();

    const status = await screen.findByTestId("executor-prepare-status");
    expect(status.dataset.phase).toBe("ready");
    expect(screen.getByText(/Environment ready/)).toBeTruthy();
    expect(screen.getByText(/13s/)).toBeTruthy();
  });
});
