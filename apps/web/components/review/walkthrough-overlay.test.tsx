import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectionStatus } from "@/lib/types/connection";
import type { TaskWalkthrough } from "@/lib/types/http";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { deleteTaskWalkthrough, getTaskWalkthrough } from "@/lib/api/domains/walkthrough-api";
import { getOpenWalkthroughTaskId, setOpenWalkthroughTaskId } from "@/lib/walkthrough-open-state";
import { WalkthroughOverlay } from "./walkthrough-overlay";

vi.mock("@/lib/api/domains/walkthrough-api", () => ({
  deleteTaskWalkthrough: vi.fn(),
  getTaskWalkthrough: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
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
    vi.mocked(deleteTaskWalkthrough).mockReset();
    setOpenWalkthroughTaskId(null);
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

  it("uses a readable selected launcher state and clears open state on close", async () => {
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    renderOverlay();
    await waitFor(() => expect(setConnectionStatus).toBeTruthy());
    act(() => setConnectionStatus?.("connected"));
    const launcher = await screen.findByTestId("walkthrough-launcher");

    fireEvent.click(launcher);

    await waitFor(() => expect(getOpenWalkthroughTaskId()).toBe(TASK_ID));
    expect(screen.getByTestId("walkthrough-floating")).toBeTruthy();
    expect(launcher.className).not.toContain("bg-accent");
    const count = within(launcher).getByText("1/1");
    expect(count.className).toContain("text-foreground");

    fireEvent.click(launcher);

    expect(getOpenWalkthroughTaskId()).toBeNull();
  });
});
