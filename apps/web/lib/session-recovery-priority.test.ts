import { describe, expect, it } from "vitest";
import { selectPrimaryRecoveryAction } from "./session-recovery-actions";

describe("primary recovery selection", () => {
  it("prefers saved-history continuation over workspace restoration", () => {
    expect(selectPrimaryRecoveryAction(["restore", "continue_from_history"])).toBe(
      "continue_from_history",
    );
  });

  it("prefers connection recovery before actions that submit another prompt", () => {
    expect(selectPrimaryRecoveryAction(["resume", "retry_connection"])).toBe("retry_connection");
  });
});
