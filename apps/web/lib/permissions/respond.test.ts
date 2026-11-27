import { describe, it, expect } from "vitest";
import {
  allowAlwaysDecision,
  approveDecision,
  buildPermissionRespondRequest,
  codexDecisionLabel,
  denyDecision,
  isStalePermissionResponse,
  offeredChoiceDecision,
  offeredChoices,
  type PermissionOption,
} from "./respond";

const ALLOW_ONCE: PermissionOption = { option_id: "allow", name: "Allow", kind: "allow_once" };
const ALLOW_ALWAYS: PermissionOption = {
  option_id: "always",
  name: "Always",
  kind: "allow_always",
};
const REJECT_ONCE: PermissionOption = { option_id: "deny", name: "Deny", kind: "reject_once" };
const REJECT_ALWAYS: PermissionOption = {
  option_id: "deny-always",
  name: "Deny always",
  kind: "reject_always",
};

function codex(id: string, decision: string, kind: PermissionOption["kind"]): PermissionOption {
  return {
    option_id: id,
    name: id,
    kind,
    metadata: { codex_app_server: true, codex_decision: decision },
  };
}

describe("isStalePermissionResponse", () => {
  it("matches the three stale codes and nothing else", () => {
    for (const code of [
      "permission_not_found",
      "permission_stale",
      "permission_already_resolved",
    ]) {
      expect(isStalePermissionResponse(new Error(`x ${code} y`))).toBe(true);
    }
    expect(isStalePermissionResponse(new Error("timeout"))).toBe(false);
    expect(isStalePermissionResponse("permission_stale")).toBe(false);
  });
});

describe("denyDecision", () => {
  it("picks the first reject-kind option in list order and marks it rejected", () => {
    expect(denyDecision([REJECT_ALWAYS, REJECT_ONCE])).toEqual({
      optionId: "deny-always",
      cancelled: false,
      rejected: true,
    });
  });

  it("falls back to the cancel decision when no reject option exists", () => {
    expect(denyDecision([ALLOW_ONCE])).toEqual({ optionId: "", cancelled: true, rejected: false });
  });
});

describe("approveDecision", () => {
  it("prefers allow_once over allow_always regardless of order", () => {
    expect(approveDecision([ALLOW_ALWAYS, ALLOW_ONCE])).toEqual({
      optionId: "allow",
      cancelled: false,
      rejected: false,
    });
  });

  it("falls back to allow_always", () => {
    expect(approveDecision([REJECT_ONCE, ALLOW_ALWAYS])?.optionId).toBe("always");
  });

  it("is null when no allow option exists", () => {
    expect(approveDecision([REJECT_ONCE])).toBeNull();
  });
});

describe("allowAlwaysDecision", () => {
  it("returns the allow_always option or null", () => {
    expect(allowAlwaysDecision([ALLOW_ONCE, ALLOW_ALWAYS])?.optionId).toBe("always");
    expect(allowAlwaysDecision([ALLOW_ONCE])).toBeNull();
  });
});

describe("codex offered choices", () => {
  const options = [
    ALLOW_ONCE,
    codex("c-accept", "accept", "allow_once"),
    codex("c-decline", "decline", "reject_once"),
    codex("c-cancel", "cancel", "reject_once"),
  ];

  it("lists only options flagged codex_app_server, with the chat labels", () => {
    expect(offeredChoices(options).map((c) => c.option_id)).toEqual([
      "c-accept",
      "c-decline",
      "c-cancel",
    ]);
    expect(codexDecisionLabel(options[2])).toBe("Deny");
  });

  it("maps a decline to rejected, a cancel to cancelled with no option id", () => {
    expect(offeredChoiceDecision(options, "c-decline")).toEqual({
      optionId: "c-decline",
      cancelled: false,
      rejected: true,
    });
    expect(offeredChoiceDecision(options, "c-cancel")).toEqual({
      optionId: "",
      cancelled: true,
      rejected: false,
    });
    expect(offeredChoiceDecision(options, "c-accept")?.rejected).toBe(false);
  });

  it("ignores options without the codex flag", () => {
    expect(offeredChoiceDecision(options, "allow")).toBeNull();
  });
});

describe("buildPermissionRespondRequest", () => {
  const identity = { task_id: "t1", session_id: "s1", request_id: "r1", pending_id: "p1" };

  it("sends the option id with both flags false for an allow", () => {
    expect(
      buildPermissionRespondRequest(identity, {
        optionId: "allow",
        cancelled: false,
        rejected: false,
      }),
    ).toEqual({ ...identity, option_id: "allow", cancelled: false, rejected: false });
  });

  it("omits the option id for a cancel", () => {
    expect(
      buildPermissionRespondRequest(identity, { optionId: "", cancelled: true, rejected: false }),
    ).toEqual({ ...identity, option_id: undefined, cancelled: true, rejected: false });
  });

  it("sends rejected true for a reject", () => {
    expect(
      buildPermissionRespondRequest(identity, {
        optionId: "deny",
        cancelled: false,
        rejected: true,
      }),
    ).toMatchObject({ option_id: "deny", rejected: true, cancelled: false });
  });
});
