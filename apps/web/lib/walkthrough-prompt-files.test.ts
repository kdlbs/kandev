import { describe, expect, it } from "vitest";
import { buildWalkthroughPromptFiles } from "./walkthrough-prompt-files";

const APP_PATH = "src/app.ts";

describe("buildWalkthroughPromptFiles", () => {
  it("includes working tree, staged, committed, and PR files in review order", () => {
    const files = buildWalkthroughPromptFiles({
      unstagedFiles: [{ path: APP_PATH }],
      stagedFiles: [{ path: "src/staged.ts", repositoryName: "web" }],
      committedFiles: {
        "src/committed.ts": { path: "src/committed.ts" },
      },
      prFiles: [{ path: "src/pr.ts", repository_name: "backend" }],
    });

    expect(files).toEqual([
      { path: APP_PATH, source: "uncommitted" },
      { path: "src/staged.ts", repository_name: "web", source: "staged" },
      { path: "src/committed.ts", source: "committed" },
      { path: "src/pr.ts", repository_name: "backend", source: "pr" },
    ]);
  });

  it("keeps the freshest local source when cumulative diff repeats a file", () => {
    const files = buildWalkthroughPromptFiles({
      unstagedFiles: [{ path: APP_PATH }],
      stagedFiles: [],
      committedFiles: {
        [APP_PATH]: { path: APP_PATH },
        "src/other.ts": { path: "src/other.ts" },
      },
      prFiles: [],
    });

    expect(files).toEqual([
      { path: APP_PATH, source: "uncommitted" },
      { path: "src/other.ts", source: "committed" },
    ]);
  });

  it("falls back to composite cumulative-diff keys when the payload lacks path fields", () => {
    const files = buildWalkthroughPromptFiles({
      unstagedFiles: [],
      stagedFiles: [],
      committedFiles: {
        "backend\u0000src/server.go": {},
      },
      prFiles: [],
    });

    expect(files).toEqual([
      { path: "src/server.go", repository_name: "backend", source: "committed" },
    ]);
  });
});
