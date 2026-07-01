import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";

// Constants declared before mocks so the mocked factories can reference them without
// hoisting-related TDZ pitfalls.
const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const STEP_CREATE_SANDBOX = "Creating cloud sandbox";
const PREPARE_STATUS_TESTID = "executor-prepare-status";
const SETTINGS_BUTTON_TESTID = "executor-settings-button";
const BRANCH_PUSH_HINT =
  "Push your branch to its remote in the confirmation dialog to preserve committed work before resetting.";
const SPRITES_ENV = { executor_type: "sprites", sandbox_id: "kandev-abc" };
const SPRITES_WORKTREE_ENV = { ...SPRITES_ENV, worktree_path: "/tmp/worktree" };
const DOCKER_ENV = { executor_type: "local_docker", container_id: "abcdef" };

type MockEnv = {
  executor_type: string;
  sandbox_id?: string;
  container_id?: string;
  worktree_path?: string;
};

let mockPrepareState: SessionPrepareState | null = null;
let mockSessionState: string | null = null;
let mockEnv: MockEnv | null = null;

const renderButton = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <ExecutorSettingsButton taskId={TASK_ID} sessionId={SESSION_ID} />
    </QueryClientProvider>,
  );
};

const hoverSettingsButton = () =>
  fireEvent.pointerEnter(screen.getByTestId(SETTINGS_BUTTON_TESTID));

const flushTicks = () => Promise.resolve().then(() => Promise.resolve());

afterEach(() => {
  cleanup();
  mockPrepareState = null;
  mockSessionState = null;
  mockEnv = null;
});

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

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { ExecutorSettingsButton } from "./executor-settings-button";

describe("ExecutorSettingsButton icon states", () => {
  it("shows the cloud icon when the executor type is sprites", async () => {
    mockEnv = SPRITES_ENV;

    renderButton();
    await flushTicks();
    await flushTicks();

    expect(await screen.findByTestId("executor-status-cloud-icon")).toBeTruthy();
  });

  it("shows the container icon for both docker variants", async () => {
    mockEnv = DOCKER_ENV;

    renderButton();
    await flushTicks();

    expect(await screen.findByTestId("executor-status-container-icon")).toBeTruthy();
  });

  it("swaps to a spinner while the prepare progress is preparing", async () => {
    mockEnv = SPRITES_ENV;
    mockPrepareState = {
      sessionId: SESSION_ID,
      status: "preparing",
      steps: [{ name: STEP_CREATE_SANDBOX, status: "running" }],
    };

    renderButton();

    expect(screen.getByTestId("executor-settings-button-spinner")).toBeTruthy();
    expect(screen.queryByTestId("executor-status-cloud-icon")).toBeNull();
  });
});

describe("ExecutorSettingsButton prepare status", () => {
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

    renderButton();
    hoverSettingsButton();

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

    renderButton();
    hoverSettingsButton();

    const status = await screen.findByTestId(PREPARE_STATUS_TESTID);
    expect(status.dataset.phase).toBe("preparing_fallback");
    expect(screen.getByTestId("executor-prepare-fallback-warning")).toBeTruthy();
  });

  it("renders the Resuming session row when session is STARTING with no prepare events", async () => {
    mockSessionState = "STARTING";

    renderButton();
    expect(screen.getByTestId("executor-settings-button-spinner")).toBeTruthy();
    hoverSettingsButton();

    const status = await screen.findByTestId(PREPARE_STATUS_TESTID);
    expect(status.dataset.phase).toBe("resuming");
    expect(screen.getByText("Resuming session")).toBeTruthy();
    expect(screen.getByText(/Reconnecting to the existing environment/)).toBeTruthy();
  });

  it("hides the prepare-status section once preparation completes", async () => {
    // The READY badge next to the executor name conveys ready-state; this dedicated
    // "Environment ready · 12s" row is redundant in the tooltip.
    mockPrepareState = {
      sessionId: SESSION_ID,
      status: "completed",
      steps: [{ name: STEP_CREATE_SANDBOX, status: "completed" }],
      durationMs: 12500,
    };

    renderButton();
    hoverSettingsButton();

    expect(screen.queryByTestId(PREPARE_STATUS_TESTID)).toBeNull();
    expect(screen.queryByText(/Environment ready/)).toBeNull();
  });
});

describe("ExecutorSettingsButton reset tooltip", () => {
  it("does not mention branch push in tooltip when no worktree path exists", async () => {
    mockEnv = SPRITES_ENV;

    renderButton();
    hoverSettingsButton();

    // Wait for env to load (cloud icon only renders once executor_type resolves)
    // and for HoverCardContent to open (reset button lives inside the popover).
    await screen.findByTestId("executor-status-cloud-icon");
    await screen.findByTestId("executor-settings-reset");

    expect(screen.queryByText(BRANCH_PUSH_HINT)).toBeNull();
  });

  it("mentions branch push in tooltip when a worktree path exists", async () => {
    mockEnv = SPRITES_WORKTREE_ENV;

    renderButton();
    hoverSettingsButton();

    expect(await screen.findByText(BRANCH_PUSH_HINT)).toBeTruthy();
  });
});
