import { describe, expect, it } from "vitest";
import { computeHasAllBranches } from "./task-create-dialog-prop-builders";
import type { DialogFormState } from "@/components/task-create-dialog-types";

// Minimal fs stub for computeHasAllBranches. The function only reads
// `noRepository`, `useGitHubUrl`, `githubBranch`, and `repositories[].branch`,
// so we cast a partial through `unknown` to avoid having to materialise the
// full DialogFormState surface in tests.
function fsStub(overrides: {
  noRepository?: boolean;
  useGitHubUrl?: boolean;
  githubBranch?: string;
  repositories?: Array<{ branch?: string }>;
}): DialogFormState {
  return {
    noRepository: false,
    useGitHubUrl: false,
    githubBranch: "",
    repositories: [],
    ...overrides,
  } as unknown as DialogFormState;
}

describe("computeHasAllBranches", () => {
  it("returns true when the task is in no-repository mode (short-circuits the rest)", () => {
    // noRepository mode doesn't need a branch — the backend skips the
    // worktree/clone step entirely, so it always satisfies hasAllBranches.
    expect(computeHasAllBranches(fsStub({ noRepository: true, repositories: [] }))).toBe(true);
  });

  it("treats the GitHub-URL mode as branched iff githubBranch is non-empty", () => {
    expect(computeHasAllBranches(fsStub({ useGitHubUrl: true, githubBranch: "" }))).toBe(false);
    expect(computeHasAllBranches(fsStub({ useGitHubUrl: true, githubBranch: "main" }))).toBe(true);
  });

  it("requires every selected repository row to carry a branch", () => {
    expect(computeHasAllBranches(fsStub({ repositories: [] }))).toBe(false);
    expect(computeHasAllBranches(fsStub({ repositories: [{ branch: "main" }] }))).toBe(true);
    expect(
      computeHasAllBranches(fsStub({ repositories: [{ branch: "main" }, { branch: "" }] })),
    ).toBe(false);
    expect(
      computeHasAllBranches(
        fsStub({ repositories: [{ branch: "main" }, { branch: "feature/x" }] }),
      ),
    ).toBe(true);
  });
});
