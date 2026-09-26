import { describe, expect, it } from "vitest";
import { buildAllFiles } from "./review-dialog";

const CHILD_SCOPE = "vendor/lib";
const README_PATH = "README.md";

describe("buildAllFiles submodule metadata", () => {
  it("retains submodule metadata on PR-only files", () => {
    const result = buildAllFiles(
      null,
      null,
      [
        {
          filename: README_PATH,
          status: "modified",
          patch: "@@ -1 +1 @@\n-old\n+new\n",
          additions: 1,
          deletions: 1,
          is_submodule: true,
        },
      ],
      CHILD_SCOPE,
    );

    expect(result).toEqual([
      expect.objectContaining({
        path: README_PATH,
        repository_name: CHILD_SCOPE,
        source: "pr",
        is_submodule: true,
      }),
    ]);
  });
});
