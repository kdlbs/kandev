import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { getProposalConflict } from "./coordinator-api";

const proposal = { id: "p-1", kind: "answer", status: "rejected", error: "stale" };

describe("getProposalConflict with a refused answer", () => {
  it("reads the proposal from the refused 409 body", () => {
    const error = new ApiError("refused", 409, { error: "refused", code: "stale", proposal });
    expect(getProposalConflict(error)).toEqual(proposal);
  });

  it("still reads the standard conflict body", () => {
    const error = new ApiError("c", 409, { error_code: "proposal_conflict", proposal });
    expect(getProposalConflict(error)).toEqual(proposal);
  });

  it("ignores a 409 of another kind", () => {
    const error = new ApiError("c", 409, { error: "policy_denied" });
    expect(getProposalConflict(error)).toBeNull();
  });
});
