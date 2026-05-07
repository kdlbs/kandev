import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";

// Constants declared *above* the vi.mock factories so the mocks can reference
// them without relying on vitest's hoisting capturing TDZ-undefined closures.
// The previous layout worked by accident (factories aren't invoked until the
// test imports the module under test, by which time the const initialization
// has run), but a refactor that imports the module earlier — for example, a
// shared test helper — would break the closure silently.
const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const STEP_CREATE_SANDBOX = "Creating cloud sandbox";
const PREPARE_STATUS_TESTID = "executor-prepare-status";
const SETTINGS_BUTTON_TESTID = "executor-settings-button";

let mockPrepareState: SessionPrepareState | null = null;
let mockSessionState: string | null = null;
let mockEnv: { executor_type: string; sandbox_id?: string; container_id?: string } | null = null;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      prepareProgress: {
        bySessionId: mockPrepareState ? { [mockPrepareState.sessionId]: mockPrepareState } : {},
      },
      taskSessions: {
        items: mockSessionState
          ? { [SESSION_ID]: { id: SESSION_ID, state: mockSessionState } }
          : {},
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

describe("ExecutorSettingsButton", () => {
  afterEach(() => {
    cleanup();
    mockPrepareState = null;
    mockSessionState = null;
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

    // Open the hover card by hovering the trigger (not clicking — it's a
    // borderless info surface, not a button).
    fireEvent.pointerEnter(screen.getByTestId(SETTINGS_BUTTON_TESTID));

    expect(await screen.findByTestId(PREPARE_STATUS_TESTID)).toHaveProperty(
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
    fireEvent.pointerEnter(screen.getByTestId(SETTINGS_BUTTON_TESTID));

    const status = await screen.findByTestId(PREPARE_STATUS_TESTID);
    expect(status.dataset.phase).toBe("preparing_fallback");
    expect(screen.getByTestId("executor-prepare-fallback-warning")).toBeTruthy();
  });

  it("renders the Resuming session row when session is STARTING with no prepare events", async () => {
    mockSessionState = "STARTING";

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);

    expect(screen.getByTestId("executor-settings-button-spinner")).toBeTruthy();
    fireEvent.pointerEnter(screen.getByTestId(SETTINGS_BUTTON_TESTID));

    const status = await screen.findByTestId(PREPARE_STATUS_TESTID);
    expect(status.dataset.phase).toBe("resuming");
    expect(screen.getByText("Resuming session")).toBeTruthy();
    expect(screen.getByText(/Reconnecting to the existing environment/)).toBeTruthy();
  });

  it("hides the prepare-status section once preparation completes", async () => {
    // The READY badge next to the executor name conveys ready-state; the
    // dedicated "Environment ready · 12s" row is redundant noise.
    mockPrepareState = {
      sessionId: SESSION_ID,
      status: "completed",
      steps: [{ name: "Creating cloud sandbox", status: "completed" }],
      durationMs: 12500,
    };

    render(<ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />);
    fireEvent.pointerEnter(screen.getByTestId(SETTINGS_BUTTON_TESTID));

    expect(screen.queryByTestId(PREPARE_STATUS_TESTID)).toBeNull();
    expect(screen.queryByText(/Environment ready/)).toBeNull();
  });
});
