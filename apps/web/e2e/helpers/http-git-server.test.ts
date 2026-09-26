import { execFile, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";

import { describe, expect, it } from "vitest";

import { startHTTPGitFixture } from "./http-git-server";

const execFileAsync = promisify(execFile);

describe("startHTTPGitFixture", () => {
  it("advertises a commit pushed after startup over dumb HTTP", async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-"));
    let fixture: Awaited<ReturnType<typeof startHTTPGitFixture>> | undefined;

    try {
      let port = 0;
      const gitEnv = {
        ...process.env,
        GIT_AUTHOR_NAME: "E2E Test",
        GIT_AUTHOR_EMAIL: "e2e@test.local",
        GIT_COMMITTER_NAME: "E2E Test",
        GIT_COMMITTER_EMAIL: "e2e@test.local",
      };
      fixture = await startHTTPGitFixture(root, "mutable", {
        bridgeGateway: "127.0.0.1",
        onListening: (_server, listeningPort) => {
          port = listeningPort;
        },
      });
      const bare = path.join(root, "fixture", "mutable.git");
      fs.rmSync(path.join(bare, "hooks", "post-update"));
      fs.writeFileSync(path.join(fixture.checkoutPath, "after-start.txt"), "fresh\n");
      execFileSync("git", ["add", "after-start.txt"], { cwd: fixture.checkoutPath });
      execFileSync("git", ["commit", "-m", "add fixture file"], {
        cwd: fixture.checkoutPath,
        env: gitEnv,
      });
      execFileSync("git", ["remote", "set-url", "origin", bare], { cwd: fixture.checkoutPath });
      execFileSync("git", ["push", "origin", "main"], { cwd: fixture.checkoutPath });

      const clone = path.join(root, "clone");
      await execFileAsync("git", ["clone", `http://127.0.0.1:${port}/fixture/mutable.git`, clone]);

      expect(fs.readFileSync(path.join(clone, "after-start.txt"), "utf8")).toBe("fresh\n");
    } finally {
      await fixture?.close();
      fs.rmSync(root, { recursive: true, force: true });
    }
  });

  it("supports shallow clones over HTTP", async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-"));
    let fixture: Awaited<ReturnType<typeof startHTTPGitFixture>> | undefined;

    try {
      let port = 0;
      fixture = await startHTTPGitFixture(root, "shallow", {
        bridgeGateway: "127.0.0.1",
        onListening: (_server, listeningPort) => {
          port = listeningPort;
        },
      });

      const clone = path.join(root, "clone");
      await execFileAsync("git", [
        "clone",
        "--depth=1",
        `http://127.0.0.1:${port}/fixture/shallow.git`,
        clone,
      ]);

      expect(fs.readFileSync(path.join(clone, "remote-source.txt"), "utf8")).toBe(
        "shallow fixture\n",
      );
    } finally {
      await fixture?.close();
      fs.rmSync(root, { recursive: true, force: true });
    }
  });

  it("returns not found for a missing smart HTTP repository", async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-"));
    let fixture: Awaited<ReturnType<typeof startHTTPGitFixture>> | undefined;

    try {
      let port = 0;
      fixture = await startHTTPGitFixture(root, "existing", {
        bridgeGateway: "127.0.0.1",
        onListening: (_server, listeningPort) => {
          port = listeningPort;
        },
      });

      const response = await fetch(
        `http://127.0.0.1:${port}/fixture/missing.git/info/refs?service=git-upload-pack`,
      );

      expect(response.status).toBe(404);
    } finally {
      await fixture?.close();
      fs.rmSync(root, { recursive: true, force: true });
    }
  });
});
