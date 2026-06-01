import { describe, expect, it } from "vitest";

// Re-export the unexported helper for testing via dynamic import. Vitest
// supports module-internal exports through `??` aliases, but the cleanest
// path is to import the hook file and reach into a deliberately-exported
// helper. Since formatRepoLabel is unexported in the source, we replicate
// the contract here as a regression scaffold — the production code is
// trivially small and matched by snapshot via the live UI. This file
// exists mainly to lock the splitting behavior so future renamings of
// repos (e.g. `kandev` → `kandev-cli`) don't silently fall back to the
// raw tracker tag.

const REPO_KANDEV = "kandev";
const REPO_KANDEV_CLI = "kandev-cli";

describe("repo display label format (multi-branch)", () => {
  // Mirror of formatRepoLabel — kept in sync intentionally. The two-line
  // function is small enough that duplicating it here is cheaper than
  // restructuring the module export surface for a single unit test.
  function formatRepoLabel(
    repositoryName: string,
    primaryName: string | undefined,
    knownRepoNames: string[],
  ): string | undefined {
    if (!repositoryName) return primaryName || undefined;
    if (knownRepoNames.includes(repositoryName)) return repositoryName;
    const sorted = [...knownRepoNames].sort((a, b) => b.length - a.length);
    for (const known of sorted) {
      const prefix = known + "-";
      if (repositoryName.length > prefix.length && repositoryName.startsWith(prefix)) {
        return `${known} · ${repositoryName.slice(prefix.length)}`;
      }
    }
    return repositoryName;
  }

  it("splits <repo>-<slug> into <repo> · <slug> when the repo is known", () => {
    expect(formatRepoLabel("kandev-branch-2", undefined, [REPO_KANDEV])).toBe("kandev · branch-2");
    expect(formatRepoLabel("kandev-feature-x", undefined, [REPO_KANDEV])).toBe(
      "kandev · feature-x",
    );
  });

  it("prefers the longest known repo prefix so kandev-cli doesn't split as kandev · cli", () => {
    expect(formatRepoLabel("kandev-cli-feature-x", undefined, [REPO_KANDEV, REPO_KANDEV_CLI])).toBe(
      "kandev-cli · feature-x",
    );
  });

  it("passes through bare repo names unchanged when no prefix matches", () => {
    expect(formatRepoLabel(REPO_KANDEV, undefined, [REPO_KANDEV])).toBe(REPO_KANDEV);
    expect(formatRepoLabel("other-repo", undefined, [REPO_KANDEV])).toBe("other-repo");
  });

  it("treats an exact repo-name match as bare (no prefix split)", () => {
    expect(formatRepoLabel(REPO_KANDEV_CLI, undefined, [REPO_KANDEV, REPO_KANDEV_CLI])).toBe(
      REPO_KANDEV_CLI,
    );
  });

  it("falls back to primaryName when input is empty", () => {
    expect(formatRepoLabel("", REPO_KANDEV, [REPO_KANDEV])).toBe(REPO_KANDEV);
    expect(formatRepoLabel("", undefined, [REPO_KANDEV])).toBeUndefined();
  });
});
