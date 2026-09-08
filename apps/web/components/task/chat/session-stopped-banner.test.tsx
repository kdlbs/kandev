import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SessionStoppedBannerProps } from "./session-stopped-banner";
import { WebSocketRequestError } from "@/lib/ws/client";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  agentProfiles: [{ id: "profile-1" }],
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: mocks.agentProfiles },
      taskSessions: {
        items: {
          "session-1": { agent_profile_id: "profile-1" },
        },
      },
    }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: ({
    open,
    taskId,
  }: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    taskId: string;
  }) => (open ? <div data-testid="new-session-dialog">New agent dialog for {taskId}</div> : null),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "task:sessionCompleted": "This session is complete.",
        "task:newAgent": "New Agent",
        "task:agentHasStopped": "This agent has stopped.",
        "task:resume": "Resume",
        "task:resuming": "Resuming...",
        "task:starting": "Starting...",
        "task:startFreshSession": "Start fresh session",
        "task:agentProfileNoLongerExists": "The agent profile no longer exists.",
        "task:continueOnNewBranch": "Continue on a new branch",
        "task:restoreReadOnlyWorkspace": "Restore read-only workspace",
        "task:retry": "Retry",
        "task:couldnTStartASession": "Session recovery failed",
      })[key] ?? key,
  }),
}));

import { SessionStoppedBanner } from "./session-stopped-banner";

const baseProps: Omit<SessionStoppedBannerProps, "mode" | "onShowDialog" | "showDialog"> = {
  taskId: "task-1",
  sessionId: "session-1",
  workspaceId: "workspace-1",
};
const RESUME_BUTTON_TEST_ID = "recovery-resume-button";
const FRESH_BUTTON_TEST_ID = "recovery-fresh-button";

function BannerHarness({
  mode,
  ...overrides
}: Partial<SessionStoppedBannerProps> & { mode: "completed" | "recoverable" }) {
  const [showDialog, setShowDialog] = useState(false);
  return (
    <TooltipProvider>
      <SessionStoppedBanner
        {...baseProps}
        {...overrides}
        mode={mode}
        showDialog={showDialog}
        onShowDialog={setShowDialog}
      />
    </TooltipProvider>
  );
}

beforeEach(() => {
  mocks.request.mockReset().mockResolvedValue(undefined);
  mocks.agentProfiles.splice(0, mocks.agentProfiles.length, { id: "profile-1" });
});

afterEach(cleanup);

describe("SessionStoppedBanner basics", () => {
  it("offers only New Agent for a completed session and opens the new-session dialog", async () => {
    render(<BannerHarness mode="completed" />);

    expect(screen.getByTestId("completed-session-banner")).toBeTruthy();
    expect(screen.getByText("This session is complete.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "New Agent" })).toBeTruthy();
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(FRESH_BUTTON_TEST_ID)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "New Agent" }));

    expect(await screen.findByTestId("new-session-dialog")).toBeTruthy();
    expect(mocks.request).not.toHaveBeenCalled();
  });

  it("disables New Agent when no task is available", () => {
    const onShowDialog = vi.fn();
    render(
      <TooltipProvider>
        <SessionStoppedBanner
          mode="completed"
          showDialog={false}
          onShowDialog={onShowDialog}
          taskId={null}
          sessionId={null}
        />
      </TooltipProvider>,
    );

    const button = screen.getByRole("button", { name: "New Agent" }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);

    fireEvent.click(button);

    expect(onShowDialog).not.toHaveBeenCalled();
  });

  it("keeps resume and fresh-start recovery actions for a recoverable session", async () => {
    render(<BannerHarness mode="recoverable" />);

    fireEvent.click(screen.getByTestId(RESUME_BUTTON_TEST_ID));
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith(
        "session.recover",
        { task_id: "task-1", session_id: "session-1", action: "resume" },
        30000,
      ),
    );

    fireEvent.click(screen.getByTestId(FRESH_BUTTON_TEST_ID));
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith(
        "session.recover",
        { task_id: "task-1", session_id: "session-1", action: "fresh_start" },
        30000,
      ),
    );
  });

  it("opens the new-session dialog instead of recovering when the profile is missing", async () => {
    mocks.agentProfiles.splice(0, mocks.agentProfiles.length);
    render(<BannerHarness mode="recoverable" />);

    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID).getAttribute("disabled")).not.toBeNull();
    fireEvent.click(screen.getByTestId(FRESH_BUTTON_TEST_ID));

    expect(await screen.findByTestId("new-session-dialog")).toBeTruthy();
    expect(mocks.request).not.toHaveBeenCalled();
  });

  it("preserves executor-unavailable recovery copy and controls", () => {
    render(
      <BannerHarness
        mode="recoverable"
        message="Executor environment is unavailable"
        detail="Docker is offline"
        resumeLabel="Restart"
        resumingLabel="Restarting..."
      />,
    );

    expect(screen.getByText("Executor environment is unavailable")).toBeTruthy();
    expect(screen.getByText("(Docker is offline)")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Restart" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Start fresh session" })).toBeTruthy();
  });
});

describe("SessionStoppedBanner recovery failures", () => {
  it("keeps a typed branch error visible and offers explicit continuation", async () => {
    const branchError = new WebSocketRequestError(
      "The saved branch is no longer available.",
      "CONFLICT",
      {
        kind: "branch_unrecoverable",
        recovery_action: "resume_new_branch",
        original_branch: "feature/lost",
        base_branch: "main",
      },
    );
    mocks.request.mockRejectedValueOnce(branchError);

    render(<BannerHarness mode="recoverable" />);
    fireEvent.click(screen.getByTestId(RESUME_BUTTON_TEST_ID));

    expect(await screen.findByText("The saved branch is no longer available.")).toBeTruthy();
    expect(screen.getByTestId("recovery-new-branch-button")).toBeTruthy();
    expect(screen.getByTestId("recovery-restore-workspace-button")).toBeTruthy();

    fireEvent.click(screen.getByTestId("recovery-new-branch-button"));

    await waitFor(() =>
      expect(mocks.request).toHaveBeenLastCalledWith(
        "session.recover",
        { task_id: "task-1", session_id: "session-1", action: "resume_new_branch" },
        30000,
      ),
    );
    expect(screen.queryByText("The saved branch is no longer available.")).toBeNull();
  });

  it("does not offer branch continuation for an ordinary resume error", async () => {
    mocks.request.mockRejectedValueOnce(new Error("Provider is unavailable"));

    render(<BannerHarness mode="recoverable" />);
    fireEvent.click(screen.getByTestId(RESUME_BUTTON_TEST_ID));

    expect(await screen.findByText("Provider is unavailable")).toBeTruthy();
    expect(screen.queryByTestId("recovery-new-branch-button")).toBeNull();
    expect(screen.getByTestId("recovery-restore-workspace-button")).toBeTruthy();
  });
});
