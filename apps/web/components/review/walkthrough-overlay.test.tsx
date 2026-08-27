import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectionStatus } from "@/lib/types/connection";
import type { TaskWalkthrough } from "@/lib/types/http";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { deleteTaskWalkthrough, getTaskWalkthrough } from "@/lib/api/domains/walkthrough-api";
import { getOpenWalkthroughTaskId, setOpenWalkthroughTaskId } from "@/lib/walkthrough-open-state";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
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

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: vi.fn(),
}));

const TASK_ID = "task-1";
const SECOND_TASK_ID = "task-2";
const LAUNCHER_TEST_ID = "walkthrough-launcher";
const FLOATING_TEST_ID = "walkthrough-floating";
const DISCARD_BUTTON_TEST_ID = "walkthrough-discard";
const DISCARD_CONFIRMATION_TEST_ID = "walkthrough-discard-confirmation";
const ALERT_DIALOG_ROLE = "alertdialog";
const CANCEL_BUTTON_NAME = "Cancel";
const DISCARD_BUTTON_NAME = "Discard walkthrough";

function responsive(isFinePointer: boolean): ReturnType<typeof useResponsiveBreakpoint> {
  return {
    breakpoint: isFinePointer ? "desktop" : "mobile",
    isMobile: !isFinePointer,
    isTablet: false,
    isDesktop: isFinePointer,
    isCompactDesktop: false,
    isFullDesktop: isFinePointer,
    isFinePointer,
    usesDesktopWorkbench: isFinePointer,
  };
}

function walkthrough(taskId = TASK_ID): TaskWalkthrough {
  return {
    id: `walkthrough-${taskId}`,
    task_id: taskId,
    title: "Walkthrough",
    created_by: "agent",
    created_at: "2026-07-07T12:00:00Z",
    updated_at: "2026-07-07T12:00:00Z",
    steps: [{ file: "src/example.ts", line: 3, text: "Read this line." }],
  };
}

let setConnectionStatus: ((status: ConnectionStatus) => void) | null = null;
let setActiveTask: ((taskId: string) => void) | null = null;
let setStoredWalkthrough: ((taskId: string, walkthrough: TaskWalkthrough | null) => void) | null =
  null;

function StoreProbe() {
  setConnectionStatus = useAppStore((s) => s.setConnectionStatus);
  setActiveTask = useAppStore((s) => s.setActiveTask);
  setStoredWalkthrough = useAppStore((s) => s.setWalkthrough);
  return null;
}

function ActiveTaskOverlay() {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  return <WalkthroughOverlay taskId={taskId} />;
}

function renderOverlay({ followActiveTask = false } = {}) {
  setConnectionStatus = null;
  setStoredWalkthrough = null;
  return render(
    <StateProvider
      initialState={{
        tasks: {
          activeTaskId: TASK_ID,
          activeSessionId: null,
          pinnedSessionId: null,
          lastSessionByTaskId: {},
          resumeSkippedSessionIds: {},
        },
      }}
    >
      <StoreProbe />
      {followActiveTask ? <ActiveTaskOverlay /> : <WalkthroughOverlay taskId={TASK_ID} />}
    </StateProvider>,
  );
}

function resetOverlayMocks() {
  vi.mocked(getTaskWalkthrough).mockReset();
  vi.mocked(deleteTaskWalkthrough).mockReset();
  vi.mocked(useResponsiveBreakpoint).mockReturnValue(responsive(true));
  setOpenWalkthroughTaskId(null);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("WalkthroughOverlay", () => {
  beforeEach(resetOverlayMocks);

  it("waits for websocket connection before backfilling the launcher", async () => {
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());

    renderOverlay();

    await waitFor(() => expect(setConnectionStatus).toBeTruthy());
    expect(getTaskWalkthrough).not.toHaveBeenCalled();

    act(() => setConnectionStatus?.("connected"));

    expect(await screen.findByTestId(LAUNCHER_TEST_ID)).toBeTruthy();
    expect(getTaskWalkthrough).toHaveBeenCalledWith(TASK_ID);
  });

  it("uses a readable selected launcher state and clears open state on close", async () => {
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    renderOverlay();
    await waitFor(() => expect(setConnectionStatus).toBeTruthy());
    act(() => setConnectionStatus?.("connected"));
    const launcher = await screen.findByTestId(LAUNCHER_TEST_ID);

    fireEvent.click(launcher);

    await waitFor(() => expect(getOpenWalkthroughTaskId()).toBe(TASK_ID));
    expect(screen.getByTestId(FLOATING_TEST_ID)).toBeTruthy();
    expect(launcher.className).not.toContain("bg-accent");
    const count = within(launcher).getByText("1/1");
    expect(count.className).toContain("text-foreground");

    fireEvent.click(launcher);

    expect(getOpenWalkthroughTaskId()).toBeNull();
  });

  it("closes the walkthrough when the active task changes", async () => {
    vi.mocked(getTaskWalkthrough).mockImplementation(async (taskId) =>
      taskId === TASK_ID ? walkthrough() : null,
    );
    const view = renderOverlay({ followActiveTask: true });

    act(() => setConnectionStatus?.("connected"));
    fireEvent.click(await within(view.container).findByTestId(LAUNCHER_TEST_ID));
    await waitFor(() => expect(getOpenWalkthroughTaskId()).toBe(TASK_ID));

    act(() => setActiveTask?.(SECOND_TASK_ID));

    await waitFor(() => expect(getOpenWalkthroughTaskId()).toBeNull());
  });
});

describe("WalkthroughOverlay discard task binding", () => {
  beforeEach(resetOverlayMocks);

  it("keeps discard bound to the task that opened the confirmation", async () => {
    vi.mocked(getTaskWalkthrough).mockImplementation(async (taskId) => walkthrough(taskId));
    vi.mocked(deleteTaskWalkthrough).mockResolvedValue(undefined);
    renderOverlay({ followActiveTask: true });
    act(() => {
      setStoredWalkthrough?.(SECOND_TASK_ID, walkthrough(SECOND_TASK_ID));
      setConnectionStatus?.("connected");
    });

    await screen.findByTestId(LAUNCHER_TEST_ID);
    fireEvent.click(screen.getByTestId(DISCARD_BUTTON_TEST_ID));
    await screen.findByTestId(DISCARD_CONFIRMATION_TEST_ID);

    act(() => setActiveTask?.(SECOND_TASK_ID));

    await screen.findByTestId(LAUNCHER_TEST_ID);
    await waitFor(() => expect(screen.queryByTestId(DISCARD_CONFIRMATION_TEST_ID)).toBeNull());
    expect(deleteTaskWalkthrough).not.toHaveBeenCalled();

    act(() => setActiveTask?.(TASK_ID));

    const confirmation = await screen.findByTestId(DISCARD_CONFIRMATION_TEST_ID);
    fireEvent.click(
      within(confirmation).getByRole("button", {
        name: DISCARD_BUTTON_NAME,
      }),
    );

    await waitFor(() => expect(deleteTaskWalkthrough).toHaveBeenCalledWith(TASK_ID));
    expect(deleteTaskWalkthrough).not.toHaveBeenCalledWith(SECOND_TASK_ID);
  });
});

describe("WalkthroughOverlay fine-pointer discard confirmation", () => {
  beforeEach(resetOverlayMocks);

  it("confirms fine-pointer discard in an anchored non-modal shell", async () => {
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    renderOverlay();
    act(() => setConnectionStatus?.("connected"));

    const launcher = await screen.findByTestId(LAUNCHER_TEST_ID);
    fireEvent.click(screen.getByTestId(DISCARD_BUTTON_TEST_ID));

    const confirmation = await screen.findByTestId(DISCARD_CONFIRMATION_TEST_ID);
    expect(confirmation.getAttribute("role")).toBe("dialog");
    expect(screen.queryByRole(ALERT_DIALOG_ROLE)).toBeNull();
    expect(launcher).toBeTruthy();
  });

  it("cancels discard without closing the walkthrough", async () => {
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    renderOverlay();
    act(() => setConnectionStatus?.("connected"));

    const launcher = await screen.findByTestId(LAUNCHER_TEST_ID);
    fireEvent.click(launcher);
    await waitFor(() => expect(screen.getByTestId(FLOATING_TEST_ID)).toBeTruthy());
    fireEvent.click(screen.getByTestId(DISCARD_BUTTON_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId(DISCARD_CONFIRMATION_TEST_ID)).getByRole("button", {
        name: CANCEL_BUTTON_NAME,
      }),
    );

    await waitFor(() => expect(screen.queryByTestId(DISCARD_CONFIRMATION_TEST_ID)).toBeNull());
    expect(screen.getByTestId(FLOATING_TEST_ID)).toBeTruthy();
    expect(getOpenWalkthroughTaskId()).toBe(TASK_ID);
  });

  it("closes the local shell before discarding once", async () => {
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    let shellClosedBeforeDelete = false;
    vi.mocked(deleteTaskWalkthrough).mockImplementation(async () => {
      shellClosedBeforeDelete = screen.queryByTestId(DISCARD_CONFIRMATION_TEST_ID) === null;
    });
    renderOverlay();
    act(() => setConnectionStatus?.("connected"));

    await screen.findByTestId(LAUNCHER_TEST_ID);
    fireEvent.click(screen.getByTestId(DISCARD_BUTTON_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId(DISCARD_CONFIRMATION_TEST_ID)).getByRole("button", {
        name: DISCARD_BUTTON_NAME,
      }),
    );

    await waitFor(() => expect(deleteTaskWalkthrough).toHaveBeenCalledTimes(1));
    expect(shellClosedBeforeDelete).toBe(true);
    expect(screen.queryByTestId(DISCARD_CONFIRMATION_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(LAUNCHER_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(FLOATING_TEST_ID)).toBeNull();
  });
});

describe("WalkthroughOverlay coarse-pointer discard confirmation", () => {
  beforeEach(resetOverlayMocks);

  it("morphs coarse-pointer discard into local touch actions", async () => {
    vi.mocked(useResponsiveBreakpoint).mockReturnValue(responsive(false));
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    renderOverlay();
    act(() => setConnectionStatus?.("connected"));

    await screen.findByTestId(LAUNCHER_TEST_ID);
    fireEvent.click(screen.getByTestId(DISCARD_BUTTON_TEST_ID));

    const confirmation = await screen.findByTestId(DISCARD_CONFIRMATION_TEST_ID);
    expect(confirmation.getAttribute("role")).toBe("group");
    expect(screen.queryByRole(ALERT_DIALOG_ROLE)).toBeNull();
    expect(
      within(confirmation).getByRole("button", { name: DISCARD_BUTTON_NAME }).className,
    ).toContain("h-11");
  });

  it("restores focus to the coarse-pointer discard button after cancel", async () => {
    let runAnimationFrame: FrameRequestCallback | null = null;
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      runAnimationFrame = callback;
      return 0;
    });
    vi.mocked(useResponsiveBreakpoint).mockReturnValue(responsive(false));
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    renderOverlay();
    act(() => setConnectionStatus?.("connected"));

    await screen.findByTestId(LAUNCHER_TEST_ID);
    fireEvent.click(screen.getByTestId(DISCARD_BUTTON_TEST_ID));
    fireEvent.click(
      within(await screen.findByTestId(DISCARD_CONFIRMATION_TEST_ID)).getByRole("button", {
        name: CANCEL_BUTTON_NAME,
      }),
    );
    act(() => runAnimationFrame?.(0));

    expect(document.activeElement).toBe(screen.getByTestId(DISCARD_BUTTON_TEST_ID));
  });
});

describe("WalkthroughOverlay discard mutation state", () => {
  beforeEach(resetOverlayMocks);

  it("disables repeated discard while deletion is in flight", async () => {
    let finishDelete: (() => void) | null = null;
    vi.mocked(getTaskWalkthrough).mockResolvedValue(walkthrough());
    vi.mocked(deleteTaskWalkthrough).mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishDelete = resolve;
        }),
    );
    renderOverlay();
    act(() => setConnectionStatus?.("connected"));

    await screen.findByTestId(LAUNCHER_TEST_ID);
    fireEvent.click(screen.getByTestId(DISCARD_BUTTON_TEST_ID));
    fireEvent.click(
      within(await screen.findByTestId(DISCARD_CONFIRMATION_TEST_ID)).getByRole("button", {
        name: DISCARD_BUTTON_NAME,
      }),
    );

    await waitFor(() => expect(deleteTaskWalkthrough).toHaveBeenCalledTimes(1));
    const disabledDuringDelete = (screen.getByTestId(DISCARD_BUTTON_TEST_ID) as HTMLButtonElement)
      .disabled;

    act(() => finishDelete?.());
    await waitFor(() => expect(screen.queryByTestId(LAUNCHER_TEST_ID)).toBeNull());
    expect(disabledDuringDelete).toBe(true);
  });
});
