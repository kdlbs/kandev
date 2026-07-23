import { expect, test } from "vitest";
import { startHTTPGitFixture } from "../e2e/helpers/http-git-server";
import { execFile } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

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
      const gitConfig = Object.fromEntries(
        fixture.gitConfigEnvVars.map(({ key, value }) => [key, value]),
      );
      const { stdout: refs } = await execFileAsync(
        "git",
        ["ls-remote", "--heads", fixture.remoteURL],
        {
          env: { ...process.env, ...gitConfig },
        },
      );
      expect(refs).toMatch(/refs\/heads\/main/);
    } finally {
      await fixture.close();
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
