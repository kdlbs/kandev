import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

let mockSteps: PrepareStepInfo[] = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      prepareProgress: {
        bySessionId: {
          "session-1": {
            sessionId: "session-1",
            status: "preparing",
            steps: mockSteps,
          },
        },
      },
      taskSessions: {
        items: {
          "session-1": {
            id: "session-1",
            state: "STARTING",
          },
        },
      },
      sessionAgentctl: {
        itemsBySessionId: {},
      },
    }),
}));

import { PrepareProgress } from "./prepare-progress";

describe("PrepareProgress", () => {
  it("hides skipped steps that have no useful details", () => {
    mockSteps = [
      {
        name: "Uploading credentials",
        status: "skipped",
      },
      {
        name: "Waiting for agent controller",
        status: "completed",
      },
    ];

    render(<PrepareProgress sessionId="session-1" />);

    expect(screen.queryByText("Uploading credentials")).toBeNull();
    expect(screen.getByText("Waiting for agent controller")).toBeTruthy();
  });
});
