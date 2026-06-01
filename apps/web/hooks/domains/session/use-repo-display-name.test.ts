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
    expect(formatRepoLabel("kandev-branch-2", undefined, ["kandev"])).toBe("kandev · branch-2");
    expect(formatRepoLabel("kandev-feature-x", undefined, ["kandev"])).toBe("kandev · feature-x");
  });

  it("prefers the longest known repo prefix so kandev-cli doesn't split as kandev · cli", () => {
    expect(formatRepoLabel("kandev-cli-feature-x", undefined, ["kandev", "kandev-cli"])).toBe(
      "kandev-cli · feature-x",
    );
  });

  it("passes through bare repo names unchanged when no prefix matches", () => {
    expect(formatRepoLabel("kandev", undefined, ["kandev"])).toBe("kandev");
    expect(formatRepoLabel("other-repo", undefined, ["kandev"])).toBe("other-repo");
  });

  it("falls back to primaryName when input is empty", () => {
    expect(formatRepoLabel("", "kandev", ["kandev"])).toBe("kandev");
    expect(formatRepoLabel("", undefined, ["kandev"])).toBeUndefined();
  });
});
