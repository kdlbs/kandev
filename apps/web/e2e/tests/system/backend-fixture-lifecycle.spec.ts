import assert from "node:assert/strict";
import { type ChildProcess } from "node:child_process";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "@playwright/test";

import { runOwnedBackendFixture, waitForHealth } from "../../fixtures/backend";

test.describe("backend fixture lifecycle", () => {
  test("fails fast when the backend exited before health polling began", async () => {
    const exitedProcess = Object.assign(new EventEmitter(), {
      exitCode: 17,
      signalCode: null,
    }) as unknown as ChildProcess;

    await assert.rejects(
      waitForHealth("http://127.0.0.1:1/health", 50, exitedProcess),
      /Backend process exited with code 17/,
    );
  });

  test("fails fast when the backend process cannot spawn", async () => {
    const failedProcess = Object.assign(new EventEmitter(), {
      exitCode: null,
      signalCode: null,
    }) as unknown as ChildProcess;
    const health = waitForHealth("http://127.0.0.1:1/health", 5_000, failedProcess);

    queueMicrotask(() => failedProcess.emit("error", new Error("spawn failed")));

    await assert.rejects(health, /spawn failed/);
  });

  test("removes its exact temporary root when setup fails before process spawn", async () => {
    const parentDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-lifecycle-test-"));
    const ownedDir = fs.mkdtempSync(path.join(parentDir, "owned-"));
    const peerDir = fs.mkdtempSync(path.join(parentDir, "peer-"));
    fs.writeFileSync(path.join(ownedDir, "partial.db"), "partial");
    fs.writeFileSync(path.join(peerDir, "keep.db"), "keep");

    try {
      await assert.rejects(
        runOwnedBackendFixture(ownedDir, async () => {
          throw new Error("setup failed");
        }),
        /setup failed/,
      );

      assert.equal(fs.existsSync(ownedDir), false);
      assert.equal(fs.readFileSync(path.join(peerDir, "keep.db"), "utf8"), "keep");
    } finally {
      fs.rmSync(parentDir, { recursive: true, force: true });
    }
  });

  test("removes its temporary root after successful fixture use", async () => {
    const ownedDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-lifecycle-success-"));

    await runOwnedBackendFixture(ownedDir, async () => {
      fs.writeFileSync(path.join(ownedDir, "kandev.db"), "complete");
    });

    assert.equal(fs.existsSync(ownedDir), false);
  });

  test("stops the registered process before cleanup after a health failure", async () => {
    const events: string[] = [];
    const child = new EventEmitter() as unknown as ChildProcess;

    await assert.rejects(
      runOwnedBackendFixture(
        "/tmp/exact-owned-root",
        async (registerProcess) => {
          registerProcess(child);
          events.push("run");
          throw new Error("health failed");
        },
        {
          stopProcess: async (registeredChild) => {
            assert.equal(registeredChild, child);
            events.push("stop");
          },
          removeTempRoot: (tmpDir) => {
            assert.equal(tmpDir, "/tmp/exact-owned-root");
            events.push("remove");
          },
        },
      ),
      /health failed/,
    );

    assert.deepEqual(events, ["run", "stop", "remove"]);
  });

  test("surfaces cleanup failure alongside the fixture failure", async () => {
    const fixtureFailure = new Error("fixture failed");
    const cleanupFailure = new Error("cleanup failed");

    await assert.rejects(
      runOwnedBackendFixture(
        "/tmp/exact-owned-root",
        async () => {
          throw fixtureFailure;
        },
        {
          removeTempRoot: () => {
            throw cleanupFailure;
          },
        },
      ),
      (error) => {
        assert.ok(error instanceof AggregateError);
        assert.equal(error.errors[0], fixtureFailure);
        assert.match(error.errors[1].message, /Failed to remove E2E temporary root/);
        assert.equal(error.errors[1].cause, cleanupFailure);
        return true;
      },
    );
  });
});
