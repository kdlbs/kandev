import { describe, expect, it } from "vitest";
import { sandboxErrorMessageKey, sandboxMessageKey } from "./sandbox-messages";
import { ApiError } from "./api/client";

describe("sandbox diagnostic messages", () => {
  it("uses only known HTTP error codes and never raw error text", () => {
    expect(
      sandboxErrorMessageKey(new ApiError("private", 409, { error: "sandbox_revision_conflict" })),
    ).toBe("agents:sandboxRevisionConflict");
    expect(sandboxErrorMessageKey(new Error("private"))).toBe(loadFailedKey);
    expect(sandboxErrorMessageKey(new ApiError("private", 500, { error: "private" }))).toBe(
      loadFailedKey,
    );
  });
  it("maps stable blockers without reflecting provider text or credentials", () => {
    expect(sandboxMessageKey("sandbox_policy_ineligible")).toBe("agents:sandboxSelectionBlocked");
    expect(sandboxMessageKey("sandbox_revision_conflict")).toBe("agents:sandboxRevisionConflict");
    expect(sandboxMessageKey("sandbox_runtime_unavailable")).toBe(
      "agents:sandboxRuntimeUnavailable",
    );
    expect(sandboxMessageKey("sandbox_policy_not_selected")).toBe(
      "agents:sandboxPolicyNotSelected",
    );
    expect(sandboxMessageKey("sandbox_operator_required")).toBe("agents:sandboxOperatorRequired");
    expect(sandboxMessageKey("secret=/private/key")).toBe(loadFailedKey);
    expect(sandboxMessageKey({ error: "secret" })).toBe(loadFailedKey);
  });
});

const loadFailedKey = "agents:sandboxLoadFailed";
