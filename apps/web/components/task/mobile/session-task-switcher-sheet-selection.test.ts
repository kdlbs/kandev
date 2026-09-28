import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/services/session-launch-service", () => ({ launchSession: vi.fn() }));
vi.mock("@/lib/session/session-auto-provisioning-fence", () => ({
  clearTaskSessionAutoProvisioningSuppression: vi.fn(),
}));

import { launchSession, type LaunchSessionResponse } from "@/lib/services/session-launch-service";
import { clearTaskSessionAutoProvisioningSuppression } from "@/lib/session/session-auto-provisioning-fence";
import {
  createTaskSheetSelectionController,
  selectTaskFromSheet,
} from "./session-task-switcher-sheet-selection";

async function flushSelection(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

const PREPARED_TASK_ID = "prepared-task";

describe("selectTaskFromSheet", () => {
  beforeEach(() => vi.clearAllMocks());

  it("clears the bulk-removal fence after a user-requested prepare creates a session", async () => {
    vi.mocked(launchSession).mockResolvedValue({ session_id: "prepared-session" } as never);
    const setActiveSession = vi.fn();
    selectTaskFromSheet({
      taskId: PREPARED_TASK_ID,
      task: { primarySessionId: null },
      state: { lastSessionByTaskId: {}, environmentIdBySessionId: {}, taskSessionsById: {} },
      selectionController: createTaskSheetSelectionController(),
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate: vi.fn(),
      onOpenChange: vi.fn(),
    });

    await flushSelection();

    expect(clearTaskSessionAutoProvisioningSuppression).toHaveBeenCalledWith(PREPARED_TASK_ID);
    expect(setActiveSession).toHaveBeenCalledWith(PREPARED_TASK_ID, "prepared-session");
  });

  it("clears the bulk-removal fence when a successful prepare is superseded", async () => {
    let resolveLaunch: (value: LaunchSessionResponse) => void = () => {};
    vi.mocked(launchSession).mockImplementation(
      () => new Promise((resolve) => (resolveLaunch = resolve)),
    );
    const selectionController = createTaskSheetSelectionController();
    const setActiveSession = vi.fn();
    selectTaskFromSheet({
      taskId: PREPARED_TASK_ID,
      task: { primarySessionId: null },
      state: { lastSessionByTaskId: {}, environmentIdBySessionId: {}, taskSessionsById: {} },
      selectionController,
      loadTaskSessionsForTask: vi.fn(async () => []),
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate: vi.fn(),
      onOpenChange: vi.fn(),
    });
    await flushSelection();
    selectionController.invalidate();
    resolveLaunch({
      success: true,
      task_id: PREPARED_TASK_ID,
      session_id: "prepared-session",
      state: "ready",
    });
    await flushSelection();

    expect(clearTaskSessionAutoProvisioningSuppression).toHaveBeenCalledWith(PREPARED_TASK_ID);
    expect(setActiveSession).not.toHaveBeenCalled();
  });
});
