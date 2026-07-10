import { describe, expect, it } from "vitest";
import { walkthroughFileMatches, walkthroughStepMatchesFile } from "./walkthrough-match";

const WEB_MAIN = "apps/web/main.ts";
const FRONTEND = "frontend";

describe("walkthroughFileMatches", () => {
  it("matches exact paths", () => {
    expect(walkthroughFileMatches(WEB_MAIN, WEB_MAIN)).toBe(true);
  });

  it("matches path suffixes only on segment boundaries", () => {
    expect(walkthroughFileMatches(WEB_MAIN, "main.ts")).toBe(true);
    expect(walkthroughFileMatches("apps/web/domain.ts", "main.ts")).toBe(false);
    expect(walkthroughFileMatches("apps/web/foobar.ts", "bar.ts")).toBe(false);
  });

  it("uses repository names when the step supplies one", () => {
    expect(
      walkthroughStepMatchesFile(
        { path: WEB_MAIN, repository_name: FRONTEND },
        { file: "main.ts", repo: FRONTEND },
      ),
    ).toBe(true);
    expect(
      walkthroughStepMatchesFile(
        { path: WEB_MAIN, repository_name: "backend" },
        { file: "main.ts", repo: FRONTEND },
      ),
    ).toBe(false);
    expect(
      walkthroughStepMatchesFile({ path: WEB_MAIN }, { file: "main.ts", repo: FRONTEND }),
    ).toBe(false);
  });

  it("does not match repo-scoped files when the step omits repo", () => {
    expect(walkthroughStepMatchesFile({ path: WEB_MAIN }, { file: "main.ts" })).toBe(true);
    expect(
      walkthroughStepMatchesFile(
        { path: WEB_MAIN, repository_name: FRONTEND },
        { file: "main.ts" },
      ),
    ).toBe(false);
  });

  it("rejects repo-less matches when the same suffix exists in multiple repos", () => {
    const files = [
      { path: WEB_MAIN, repository_name: FRONTEND },
      { path: WEB_MAIN, repository_name: "docs" },
    ];

    expect(walkthroughStepMatchesFile(files[0], { file: "main.ts" }, files)).toBe(false);
  });
});
