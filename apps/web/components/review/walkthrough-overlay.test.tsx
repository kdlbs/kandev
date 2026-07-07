import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectionStatus } from "@/lib/types/connection";
import type { TaskWalkthrough } from "@/lib/types/http";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { getTaskWalkthrough } from "@/lib/api/domains/walkthrough-api";
import { WalkthroughOverlay } from "./walkthrough-overlay";

vi.mock("@/lib/api/domains/walkthrough-api", () => ({
  getTaskWalkthrough: vi.fn(),
}));

vi.mock("@/components/diff/walkthrough-floating-window", () => ({
  WalkthroughFloatingWindow: () => <div data-testid="walkthrough-floating" />,
}));

const TASK_ID = "task-1";

function walkthrough(): TaskWalkthrough {
  return {
    id: "walkthrough-1",
    task_id: TASK_ID,
    title: "Walkthrough",
    created_by: "agent",
    created_at: "2026-07-07T12:00:00Z",
    updated_at: "2026-07-07T12:00:00Z",
    steps: [{ file: "src/example.ts", line: 3, text: "Read this line." }],
  };
}

let setConnectionStatus: ((status: ConnectionStatus) => void) | null = null;

function StoreProbe() {
  setConnectionStatus = useAppStore((s) => s.setConnectionStatus);
  return null;
}

function renderOverlay() {
  setConnectionStatus = null;
  render(
    <StateProvider
      initialState={{
        tasks: {
          activeTaskId: TASK_ID,
          activeSessionId: null,
          pinnedSessionId: null,
          lastSessionByTaskId: {},
        },
      }}
    >
      <StoreProbe />
      <WalkthroughOverlay taskId={TASK_ID} />
    </StateProvider>,
  );
}

describe("WalkthroughOverlay", () => {
  beforeEach(() => {
    vi.mocked(getTaskWalkthrough).mockReset();
  });

  it("waits for websocket connection before backfilling the launcher", async () => {
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());

    renderOverlay();

    await waitFor(() => expect(setConnectionStatus).toBeTruthy());
    expect(getTaskWalkthrough).not.toHaveBeenCalled();

    act(() => setConnectionStatus?.("connected"));

    expect(await screen.findByTestId("walkthrough-launcher")).toBeTruthy();
    expect(getTaskWalkthrough).toHaveBeenCalledWith(TASK_ID);
  });
});
