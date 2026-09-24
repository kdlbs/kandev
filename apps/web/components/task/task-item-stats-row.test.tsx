import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { TaskItemStatsRow } from "./task-item-stats-row";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ sessionPollMode: { bySessionId: {} } }),
}));

vi.mock("@/lib/config", () => ({ isDebugUI: () => false }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

afterEach(() => cleanup());

const QUEUE = {
  session_id: "session-1",
  agent_profile_id: "profile-1",
  queued_at: "2026-09-17T10:00:00Z",
  reason: "session_capacity" as const,
  retrying: true,
};

describe("TaskItemStatsRow launch queue indicator", () => {
  it("does not activate its parent row on click or keyboard input", () => {
    const parentClick = vi.fn();
    const parentKeyDown = vi.fn();
    render(
      <div onClick={parentClick} onKeyDown={parentKeyDown}>
        <TaskItemStatsRow launchQueue={QUEUE} />
      </div>,
    );

    const indicator = screen.getByTestId("sidebar-task-launch-queue");
    fireEvent.click(indicator);
    fireEvent.keyDown(indicator, { key: "Enter" });
    fireEvent.keyDown(indicator, { key: " " });

    expect(parentClick).not.toHaveBeenCalled();
    expect(parentKeyDown).not.toHaveBeenCalled();
  });
});
