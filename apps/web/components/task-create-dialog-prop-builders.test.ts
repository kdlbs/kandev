import { describe, expect, it } from "vitest";
import { computeHasAllBranches } from "./task-create-dialog-prop-builders";
import type { DialogFormState } from "@/components/task-create-dialog-types";

const URL_AB = "github.com/a/b";

// Minimal fs stub for computeHasAllBranches. The function only reads
// `noRepository`, `useRemote`, `remoteRepos`, and `repositories[].branch`,
// so we cast a partial through `unknown` to avoid having to materialise the
// full DialogFormState surface in tests.
function fsStub(overrides: {
  noRepository?: boolean;
  useRemote?: boolean;
  remoteRepos?: Array<{ url?: string; branch?: string }>;
  repositories?: Array<{ branch?: string }>;
}): DialogFormState {
  return {
    noRepository: false,
    useRemote: false,
    remoteRepos: [],
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

  it("Remote mode: every non-empty row needs a branch", () => {
    // Empty rows (URL not yet filled in) are skipped — the user can leave a
    // half-filled chip without blocking submit, mirroring the workspace path.
    expect(computeHasAllBranches(fsStub({ useRemote: true, remoteRepos: [] }))).toBe(false);
    expect(
      computeHasAllBranches(
        fsStub({ useRemote: true, remoteRepos: [{ url: URL_AB, branch: "" }] }),
      ),
    ).toBe(false);
    expect(
      computeHasAllBranches(
        fsStub({ useRemote: true, remoteRepos: [{ url: URL_AB, branch: "main" }] }),
      ),
    ).toBe(true);
    expect(
      computeHasAllBranches(
        fsStub({
          useRemote: true,
          remoteRepos: [
            { url: URL_AB, branch: "main" },
            { url: "", branch: "" },
          ],
        }),
      ),
    ).toBe(true);
    expect(
      computeHasAllBranches(
        fsStub({
          useRemote: true,
          remoteRepos: [
            { url: URL_AB, branch: "main" },
            { url: "github.com/c/d", branch: "" },
          ],
        }),
      ),
    ).toBe(false);
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
