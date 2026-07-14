import { describe, expect, it } from "vitest";
import type { UnarchiveTaskResponse } from "@/lib/api/domains/kanban-api";
import { unarchiveToastPayload } from "./unarchive-feedback";

function response(recovery: UnarchiveTaskResponse["recovery"]): UnarchiveTaskResponse {
  return {
    success: true,
    cascade_id: "",
    unarchived_ids: ["t1"],
    skipped_ids: [],
    affected_group_ids: [],
    recovery,
  };
}

describe("unarchiveToastPayload", () => {
  it("reports success when every branch is recoverable", () => {
    const payload = unarchiveToastPayload(
      response([{ task_id: "t1", repository_id: "r1", branch: "feat/x", status: "remote" }]),
    );
    expect(payload.variant).toBe("success");
    expect(payload.description).toBe("The task has been restored.");
  });

  it("reports success when there is no recovery info at all", () => {
    const payload = unarchiveToastPayload(response([]));
    expect(payload.variant).toBe("success");
  });

  it("warns with the branch names when a branch is unrecoverable", () => {
    const payload = unarchiveToastPayload(
      response([
        { task_id: "t1", repository_id: "r1", branch: "feat/gone", status: "missing" },
        { task_id: "t1", repository_id: "r2", branch: "feat/ok", status: "local" },
      ]),
    );
    expect(payload.variant).toBeUndefined();
    expect(payload.description).toContain("feat/gone");
    expect(payload.description).not.toContain("feat/ok");
  });

  it("pluralizes when multiple branches are unrecoverable", () => {
    const payload = unarchiveToastPayload(
      response([
        { task_id: "t1", repository_id: "r1", branch: "feat/one", status: "missing" },
        { task_id: "t1", repository_id: "r2", branch: "feat/two", status: "missing" },
      ]),
    );
    expect(payload.description).toContain("Branches feat/one, feat/two no longer exist");
  });

  it("tolerates a response without a recovery field", () => {
    const payload = unarchiveToastPayload(
      response(undefined as unknown as UnarchiveTaskResponse["recovery"]),
    );
    expect(payload.variant).toBe("success");
  });
});
