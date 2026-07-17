import { describe, expect, it } from "vitest";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import { computeTriggerLabel } from "./task-create-dialog-remote-repo-chip";

function row(overrides: Partial<TaskRemoteRepoRow> = {}): TaskRemoteRepoRow {
  return { key: "remote-0", url: "", branch: "", source: "paste", ...overrides };
}

describe("computeTriggerLabel", () => {
  it("returns the empty-state placeholder when url is empty", () => {
    expect(computeTriggerLabel(row())).toBe("Pick or paste a repo");
  });

  it("returns picker fullName when source is 'picker' and metadata is present", () => {
    expect(
      computeTriggerLabel(
        row({
          url: "https://github.com/octocat/hello-world",
          source: "picker",
          provider: "github",
          fullName: "octocat/hello-world",
        }),
      ),
    ).toBe("octocat/hello-world");
  });

  it("returns short paste URLs verbatim", () => {
    const label = computeTriggerLabel(row({ url: "github.com/x/y", source: "paste" }));
    expect(label).toBe("github.com/x/y");
    expect(label).not.toContain("\u2026");
  });

  it("middle-truncates long paste URLs while preserving first and last chars", () => {
    const url = "github.com/some-very-long-org/some-very-long-repo-name";
    const label = computeTriggerLabel(row({ url, source: "paste" }));
    expect(label).toContain("\u2026");
    expect(label.length).toBeLessThan(url.length);
    expect(label.startsWith(url[0]!)).toBe(true);
    expect(label.endsWith(url[url.length - 1]!)).toBe(true);
  });

  it("strips https:// and www. prefixes from paste labels", () => {
    expect(
      computeTriggerLabel(row({ url: "https://www.github.com/foo/bar", source: "paste" })),
    ).toBe("github.com/foo/bar");
  });
});
