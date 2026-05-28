import { describe, expect, it } from "vitest";
import {
  computeHasRepositorySelection,
  computeSelectedRepoCount,
} from "./task-create-dialog-computed";
import type { DialogFormState } from "@/components/task-create-dialog-types";

// Minimal fs stub for the two pure helpers below. Each function only reads
// `noRepository`, `useRemote`, `remoteRepos`, and `repositories`, so we cast a
// partial through `unknown` to avoid having to materialise the full
// DialogFormState surface in tests.
function fsStub(overrides: {
  noRepository?: boolean;
  useRemote?: boolean;
  remoteRepos?: Array<{ url?: string; branch?: string }>;
  repositories?: Array<{ repositoryId?: string; localPath?: string; branch?: string }>;
}): DialogFormState {
  return {
    noRepository: false,
    useRemote: false,
    remoteRepos: [],
    repositories: [],
    ...overrides,
  } as unknown as DialogFormState;
}

describe("computeHasRepositorySelection", () => {
  it("returns true in no-repository mode", () => {
    expect(computeHasRepositorySelection(fsStub({ noRepository: true }))).toBe(true);
  });

  it("returns true when any workspace/local row is set", () => {
    expect(computeHasRepositorySelection(fsStub({ repositories: [{ repositoryId: "r-1" }] }))).toBe(
      true,
    );
    expect(
      computeHasRepositorySelection(fsStub({ repositories: [{ localPath: "/tmp/repo" }] })),
    ).toBe(true);
  });

  it("Remote mode: returns true when ANY row has a non-empty URL", () => {
    // First row empty, second row populated — the previous implementation
    // only inspected `remoteRepos[0]` and would have returned false here.
    expect(
      computeHasRepositorySelection(
        fsStub({
          useRemote: true,
          remoteRepos: [{ url: "" }, { url: "github.com/acme/site" }],
        }),
      ),
    ).toBe(true);
  });

  it("Remote mode: returns true when only the first row is populated", () => {
    expect(
      computeHasRepositorySelection(
        fsStub({
          useRemote: true,
          remoteRepos: [{ url: "github.com/acme/site" }],
        }),
      ),
    ).toBe(true);
  });

  it("Remote mode: returns false when every row is empty / whitespace", () => {
    expect(
      computeHasRepositorySelection(
        fsStub({
          useRemote: true,
          remoteRepos: [{ url: "" }, { url: "   " }],
        }),
      ),
    ).toBe(false);
  });

  it("returns false on the empty form", () => {
    expect(computeHasRepositorySelection(fsStub({}))).toBe(false);
  });
});

describe("computeSelectedRepoCount", () => {
  it("counts workspace/local rows", () => {
    expect(
      computeSelectedRepoCount(
        fsStub({ repositories: [{ repositoryId: "r-1" }, { localPath: "/x" }] }),
      ),
    ).toBe(2);
  });

  it("ignores empty workspace rows", () => {
    // Half-filled rows (no repositoryId, no localPath) shouldn't count
    // toward the multi-repo gate even though they exist in the array.
    expect(computeSelectedRepoCount(fsStub({ repositories: [{ branch: "main" }] }))).toBe(0);
  });

  it("Remote mode: counts non-empty URL rows alongside local rows", () => {
    // Two remote URLs — without the local rows, this alone trips the gate.
    expect(
      computeSelectedRepoCount(
        fsStub({
          useRemote: true,
          remoteRepos: [{ url: "github.com/a/b" }, { url: "github.com/c/d" }],
        }),
      ),
    ).toBe(2);

    // Mix: 1 local + 2 remote = 3 repos total.
    expect(
      computeSelectedRepoCount(
        fsStub({
          useRemote: true,
          repositories: [{ repositoryId: "r-1" }],
          remoteRepos: [{ url: "github.com/a/b" }, { url: "github.com/c/d" }],
        }),
      ),
    ).toBe(3);
  });

  it("ignores remote rows when useRemote is false", () => {
    // The Remote toggle gates the count — switching modes shouldn't smuggle
    // the URL rows into the local count.
    expect(
      computeSelectedRepoCount(
        fsStub({
          useRemote: false,
          remoteRepos: [{ url: "github.com/a/b" }, { url: "github.com/c/d" }],
        }),
      ),
    ).toBe(0);
  });

  it("trips the multi-repo gate (>1) on a 2-row Remote selection", () => {
    // Multi-row Remote selection must trigger the same executor gating as
    // a 2-workspace-repo task — the previous implementation only counted
    // workspace/local rows, so this would have stayed at 0 even with two
    // populated URLs.
    const count = computeSelectedRepoCount(
      fsStub({
        useRemote: true,
        remoteRepos: [{ url: "https://github.com/a/b" }, { url: "https://github.com/c/d" }],
      }),
    );
    expect(count).toBe(2);
    expect(count > 1).toBe(true);
  });
});
