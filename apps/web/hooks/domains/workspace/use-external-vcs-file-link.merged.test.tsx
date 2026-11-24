import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useExternalVcsFileLink } from "./use-external-vcs-file-link";
import {
  SESSION_ID,
  FIRST_BRANCH,
  SECOND_BRANCH,
  githubPR,
  repeatedTaskRepositories,
  repeatedSessionWorktrees,
  wrapper,
} from "./use-external-vcs-file-link.test-helpers";

const MERGED_HEAD_SHA = "91a3d4b5678e90f1234567890abcdef123456789a";
const OTHER_HEAD_SHA = "f8b7c6d54321a09876543210fedcba9876543210a";
const CURRENT_PATH = "src/current.ts";
const PREVIOUS_PATH = "src/previous.ts";
const SHARE_BRANCH = "feature/share";
const BASE_BRANCH = "main";

afterEach(cleanup);

// @covers AC-UI-EXTERNAL-VCS-FILE-LINKS-001.4, AC-UI-EXTERNAL-VCS-FILE-LINKS-001.8, AC-UI-EXTERNAL-VCS-FILE-LINKS-001.9
describe("useExternalVcsFileLink merged GitHub revisions", () => {
  it.each(["modified", "added", "untracked", "renamed"])(
    "uses the saved head revision and current path for a merged %s file",
    (status) => {
      const { result } = renderHook(
        () =>
          useExternalVcsFileLink({
            filePath: CURRENT_PATH,
            previousPath: PREVIOUS_PATH,
            status,
          }),
        { wrapper: wrapper({ prs: [githubPR({ state: "merged", head_sha: MERGED_HEAD_SHA })] }) },
      );

      expect(result.current).toMatchObject({ revision: MERGED_HEAD_SHA, path: CURRENT_PATH });
    },
  );

  it.each([undefined, "", "   "])("falls back to base with missing head SHA %j", (head_sha) => {
    const { result } = renderHook(() => useExternalVcsFileLink({ filePath: CURRENT_PATH }), {
      wrapper: wrapper({ prs: [githubPR({ state: "merged", head_sha })] }),
    });

    expect(result.current?.revision).toBe(BASE_BRANCH);
  });

  it("trims the saved merged revision", () => {
    const { result } = renderHook(() => useExternalVcsFileLink({ filePath: CURRENT_PATH }), {
      wrapper: wrapper({ prs: [githubPR({ state: "merged", head_sha: ` ${MERGED_HEAD_SHA} ` })] }),
    });

    expect(result.current?.revision).toBe(MERGED_HEAD_SHA);
  });

  it.each(["added", "untracked"])("omits a merged %s file without a saved revision", (status) => {
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: CURRENT_PATH, status }),
      { wrapper: wrapper({ prs: [githubPR({ state: "merged" })] }) },
    );

    expect(result.current).toBeNull();
  });

  it.each([
    { head_sha: MERGED_HEAD_SHA, status: "deleted", path: CURRENT_PATH, revision: BASE_BRANCH },
    { head_sha: undefined, status: "renamed", path: PREVIOUS_PATH, revision: BASE_BRANCH },
  ])("keeps the $status base/path rule", ({ head_sha, status, path, revision }) => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: CURRENT_PATH,
          previousPath: PREVIOUS_PATH,
          status,
        }),
      { wrapper: wrapper({ prs: [githubPR({ state: "merged", head_sha })] }) },
    );

    expect(result.current).toMatchObject({ path, revision });
  });
});

describe("useExternalVcsFileLink merged candidate evidence", () => {
  it("keeps an explicit published branch for a merged GitHub PR", () => {
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: CURRENT_PATH, publishedBranch: "explicit/review" }),
      { wrapper: wrapper({ prs: [githubPR({ state: "merged", head_sha: MERGED_HEAD_SHA })] }) },
    );

    expect(result.current?.revision).toBe("explicit/review");
  });

  it.each(["open", "closed"] as const)("preserves %s source branches even with a SHA", (state) => {
    const { result } = renderHook(() => useExternalVcsFileLink({ filePath: CURRENT_PATH }), {
      wrapper: wrapper({ prs: [githubPR({ state, head_sha: MERGED_HEAD_SHA })] }),
    });

    expect(result.current?.revision).toBe(SHARE_BRANCH);
  });

  it.each([
    { head_sha: OTHER_HEAD_SHA, state: "merged" as const },
    { head_sha: undefined, state: "merged" as const },
    { head_sha: MERGED_HEAD_SHA, state: "open" as const },
  ])("falls back for conflicting same-branch evidence %j in either order", (other) => {
    const merged = githubPR({ state: "merged", head_sha: MERGED_HEAD_SHA });
    const conflicting = githubPR({ id: "pr-other", ...other });
    for (const prs of [
      [merged, conflicting],
      [conflicting, merged],
    ]) {
      const { result, unmount } = renderHook(
        () => useExternalVcsFileLink({ filePath: CURRENT_PATH }),
        { wrapper: wrapper({ prs }) },
      );

      expect(result.current?.revision).toBe(BASE_BRANCH);
      unmount();
    }
  });

  it("deduplicates identical merged targets", () => {
    const merged = githubPR({ state: "merged", head_sha: MERGED_HEAD_SHA });
    const { result } = renderHook(() => useExternalVcsFileLink({ filePath: CURRENT_PATH }), {
      wrapper: wrapper({ prs: [merged, { ...merged, id: "pr-duplicate" }] }),
    });

    expect(result.current?.revision).toBe(MERGED_HEAD_SHA);
  });

  it.each([
    { repository_id: "other-repository" },
    { repository_id: undefined, owner: "other-owner" },
  ])("ignores merged evidence from a different repository identity %j", (identity) => {
    const { result } = renderHook(() => useExternalVcsFileLink({ filePath: CURRENT_PATH }), {
      wrapper: wrapper({
        prs: [
          githubPR({ state: "merged", head_sha: MERGED_HEAD_SHA, ...identity }),
          githubPR({ id: "pr-correct" }),
        ],
      }),
    });

    expect(result.current?.revision).toBe(SHARE_BRANCH);
  });
});

describe("useExternalVcsFileLink merged repeated repositories", () => {
  // @covers AC-UI-EXTERNAL-VCS-FILE-LINKS-001.5, AC-UI-EXTERNAL-VCS-FILE-LINKS-001.9
  it.each([
    { repositoryName: "web-feature-one", revision: MERGED_HEAD_SHA },
    { repositoryName: "web-feature-two", revision: SECOND_BRANCH },
  ])(
    "selects $repositoryName by source identity before target conversion",
    ({ repositoryName, revision }) => {
      const { result } = renderHook(
        () =>
          useExternalVcsFileLink({
            filePath: CURRENT_PATH,
            sessionId: SESSION_ID,
            repositoryName,
          }),
        {
          wrapper: wrapper({
            taskRepositories: repeatedTaskRepositories(),
            sessionWorktrees: repeatedSessionWorktrees(),
            prs: [
              githubPR({
                id: "merged-one",
                head_branch: FIRST_BRANCH,
                state: "merged",
                head_sha: MERGED_HEAD_SHA,
              }),
              githubPR({ id: "open-two", head_branch: SECOND_BRANCH, head_sha: OTHER_HEAD_SHA }),
            ],
          }),
        },
      );

      expect(result.current?.revision).toBe(revision);
    },
  );

  it("does not borrow a merged sibling's saved revision", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: CURRENT_PATH,
          sessionId: SESSION_ID,
          repositoryName: "web-feature-two",
        }),
      {
        wrapper: wrapper({
          taskRepositories: repeatedTaskRepositories(),
          sessionWorktrees: repeatedSessionWorktrees(),
          prs: [
            githubPR({ head_branch: FIRST_BRANCH, state: "merged", head_sha: MERGED_HEAD_SHA }),
          ],
        }),
      },
    );

    expect(result.current?.revision).toBe("release");
  });
});
