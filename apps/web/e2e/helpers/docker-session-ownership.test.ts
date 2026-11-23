import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, test, vi } from "vitest";

import { E2E_IMAGE_TAG, restoreAgentSessionDirectoryOwnership } from "../fixtures/docker-probe";

describe("Docker fixture ownership cleanup", () => {
  test("returns the worker's agent session directory to its host owner", () => {
    const kandevHome = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-docker-session-owner-"));
    const sessionRoot = path.join(kandevHome, "agent-sessions");
    fs.mkdirSync(sessionRoot);
    const owner = fs.statSync(kandevHome);
    const runCommand = vi.fn();

    try {
      restoreAgentSessionDirectoryOwnership(kandevHome, runCommand);

      assert.deepEqual(runCommand.mock.calls, [
        [
          "docker",
          [
            "run",
            "--rm",
            "--user",
            "0:0",
            "--volume",
            `${sessionRoot}:/agent-sessions`,
            "--entrypoint",
            "chown",
            E2E_IMAGE_TAG,
            "-R",
            "--no-dereference",
            `${owner.uid}:${owner.gid}`,
            "/agent-sessions",
          ],
        ],
      ]);
    } finally {
      fs.rmSync(kandevHome, { recursive: true, force: true });
    }
  });

  test("does not start a helper container when no session directory exists", () => {
    const kandevHome = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-docker-session-empty-"));
    const runCommand = vi.fn();
    try {
      restoreAgentSessionDirectoryOwnership(kandevHome, runCommand);
      assert.equal(runCommand.mock.calls.length, 0);
    } finally {
      fs.rmSync(kandevHome, { recursive: true, force: true });
    }
  });
});
