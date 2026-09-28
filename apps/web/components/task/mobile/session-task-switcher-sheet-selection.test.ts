import { describe, expect, it, vi } from "vitest";

vi.mock("@/lib/services/session-launch-service", () => ({ launchSession: vi.fn() }));
vi.mock("@/lib/session/session-auto-provisioning-fence", () => ({
  clearTaskSessionAutoProvisioningSuppression: vi.fn(),
}));

import { launchSession } from "@/lib/services/session-launch-service";
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

describe("selectTaskFromSheet", () => {
  it("clears the bulk-removal fence after a user-requested prepare creates a session", async () => {
    vi.mocked(launchSession).mockResolvedValue({ session_id: "prepared-session" } as never);
    const setActiveSession = vi.fn();
    selectTaskFromSheet({
      taskId: "prepared-task",
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

    expect(clearTaskSessionAutoProvisioningSuppression).toHaveBeenCalledWith("prepared-task");
    expect(setActiveSession).toHaveBeenCalledWith("prepared-task", "prepared-session");
  });
});
