import { expect, test } from "vitest";
import { startHTTPGitFixture } from "../e2e/helpers/http-git-server";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

test("uses a trusted GitLab URL with a fixture-only Git rewrite", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-test-"));
  try {
    const fixture = await startHTTPGitFixture(root, "rewrite-source");
    try {
      expect(fixture.remoteURL).toBe("https://gitlab.com/fixture/rewrite-source.git");
      expect(fixture.gitConfigEnvVars).toEqual([
        { key: "GIT_CONFIG_COUNT", value: "1" },
        {
          key: "GIT_CONFIG_KEY_0",
          value: expect.stringMatching(/^url\.http:\/\/[^/]+\/.insteadOf$/),
        },
        { key: "GIT_CONFIG_VALUE_0", value: "https://gitlab.com/" },
      ]);
    } finally {
      await fixture.close();
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
