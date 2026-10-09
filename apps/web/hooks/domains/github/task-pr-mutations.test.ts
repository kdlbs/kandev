import { describe, expect, it } from "vitest";
import {
  TASK_PR_UNLINK_NO_ACTIVE_WORKSPACE,
  taskPRUnlinkErrorDescription,
} from "./task-pr-mutations";

const SELECT_WORKSPACE = "Select workspace";
const STILL_LINKED = "The pull request is still linked.";

describe("taskPRUnlinkErrorDescription", () => {
  it("uses localized messages for the workspace guard and unknown failures", () => {
    expect(
      taskPRUnlinkErrorDescription(
        TASK_PR_UNLINK_NO_ACTIVE_WORKSPACE,
        SELECT_WORKSPACE,
        STILL_LINKED,
      ),
    ).toBe(SELECT_WORKSPACE);
    expect(taskPRUnlinkErrorDescription("unknown", SELECT_WORKSPACE, STILL_LINKED)).toBe(
      STILL_LINKED,
    );
  });

  it("preserves server errors for display", () => {
    expect(
      taskPRUnlinkErrorDescription(new Error("Network unavailable"), SELECT_WORKSPACE, "Retry"),
    ).toBe("Network unavailable");
  });
});
