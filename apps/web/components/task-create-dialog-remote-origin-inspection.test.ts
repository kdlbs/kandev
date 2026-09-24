import { describe, expect, it } from "vitest";
import type { Branch } from "@/lib/types/http";
import type { RepositoryCloneSourceState } from "@/hooks/domains/repositories/use-repository-clone-source";
import type { TaskRepositorySelection } from "./task-create-dialog-types";
import {
  effectiveRemoteOriginBranch,
  remoteOriginSelectionNeedsRecovery,
  remoteOriginSelectionIsCompatible,
} from "./task-create-dialog-remote-origin-inspection";

const origin = "https://github.com/acme/api.git";

function branch(name: string): Branch {
  return { name, type: "remote", remote: "origin" };
}

function readyState(branches: Branch[], inspectedOrigin = origin): RepositoryCloneSourceState {
  return {
    status: "ready",
    result: { ready: true, origin: inspectedOrigin, branches },
  };
}

function selection(overrides: Partial<Extract<TaskRepositorySelection, { kind: "local" }>> = {}) {
  return {
    kind: "local" as const,
    key: "row-1",
    repositoryId: "repo-1",
    branch: "feature/api",
    baseBranch: "main",
    checkoutSource: "remote_origin" as const,
    expectedOrigin: origin,
    ...overrides,
  };
}

describe("remote-origin selection compatibility", () => {
  it("accepts a restored source after fresh origin inspection", () => {
    const row = selection();

    expect(effectiveRemoteOriginBranch(row)).toBe("main");
    expect(
      remoteOriginSelectionIsCompatible(row, readyState([branch("main"), branch("feature/api")])),
    ).toBe(true);
  });

  it("rejects a deleted selected branch while keeping the row identity", () => {
    const row = selection();

    expect(remoteOriginSelectionIsCompatible(row, readyState([branch("main")]))).toBe(false);
  });

  it("rejects an origin that changed since the source was saved", () => {
    const row = selection();
    const state = readyState(
      [branch("main"), branch("feature/api")],
      "https://github.com/acme/other.git",
    );

    expect(remoteOriginSelectionIsCompatible(row, state)).toBe(false);
    expect(remoteOriginSelectionNeedsRecovery(true, row, state)).toBe(true);
    expect(remoteOriginSelectionNeedsRecovery(false, row, state)).toBe(false);
  });

  it("offers recovery when a saved branch disappears after a refresh", () => {
    const row = selection();
    expect(remoteOriginSelectionNeedsRecovery(true, row, readyState([branch("main")]))).toBe(true);
  });

  it("rejects checking and unavailable inspections", () => {
    const row = selection();

    expect(remoteOriginSelectionIsCompatible(row, { status: "checking" })).toBe(false);
    expect(
      remoteOriginSelectionIsCompatible(row, {
        status: "unavailable",
        result: { ready: false, reason: "origin_unavailable", branches: [] },
      }),
    ).toBe(false);
  });
});
