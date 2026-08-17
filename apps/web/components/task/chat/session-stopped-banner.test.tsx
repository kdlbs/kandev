import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SessionStoppedBannerProps } from "./session-stopped-banner";

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
      })[key] ?? key,
  }),
}));

import { SessionStoppedBanner } from "./session-stopped-banner";

const baseProps: Omit<SessionStoppedBannerProps, "mode" | "onShowDialog" | "showDialog"> = {
  taskId: "task-1",
  sessionId: "session-1",
  workspaceId: "workspace-1",
};

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

describe("SessionStoppedBanner", () => {
  it("offers only New Agent for a completed session and opens the new-session dialog", async () => {
    render(<BannerHarness mode="completed" />);

    expect(screen.getByTestId("completed-session-banner")).toBeTruthy();
    expect(screen.getByText("This session is complete.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "New Agent" })).toBeTruthy();
    expect(screen.queryByTestId("recovery-resume-button")).toBeNull();
    expect(screen.queryByTestId("recovery-fresh-button")).toBeNull();

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

    fireEvent.click(screen.getByTestId("recovery-resume-button"));
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith(
        "session.recover",
        { task_id: "task-1", session_id: "session-1", action: "resume" },
        30000,
      ),
    );

    fireEvent.click(screen.getByTestId("recovery-fresh-button"));
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

    expect(screen.getByTestId("recovery-resume-button").getAttribute("disabled")).not.toBeNull();
    fireEvent.click(screen.getByTestId("recovery-fresh-button"));

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
