import assert from "node:assert/strict";
import { type ChildProcess } from "node:child_process";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, test, vi } from "vitest";

import {
  observeLogFile,
  reportUnexpectedBackendFailure,
  runOwnedBackendFixture,
  waitForHealth,
} from "../fixtures/backend";
import { stopSSHServer, type SSHServerHandle } from "./ssh";

describe("backend fixture lifecycle", () => {
  test("reports a test-body failure without logging passing, skipped or expected failures", () => {
    const ownedDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-test-body-backend-"));
    fs.writeFileSync(path.join(ownedDir, "backend-process.log"), "backend disconnected");
    const output = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      for (const status of ["passed", "skipped", undefined] as const) {
        reportUnexpectedBackendFailure(ownedDir, { status, expectedStatus: "passed" });
      }
      reportUnexpectedBackendFailure(ownedDir, { status: "failed", expectedStatus: "failed" });
      assert.equal(output.mock.calls.length, 0);
      for (const status of ["failed", "timedOut", "interrupted"] as const) {
        reportUnexpectedBackendFailure(ownedDir, { status, expectedStatus: "passed" });
      }
      assert.equal(output.mock.calls.length, 3);
      assert.match(output.mock.calls.flat().join(" "), /backend disconnected/);
      assert.equal(fs.existsSync(ownedDir), true);
    } finally {
      output.mockRestore();
      fs.rmSync(ownedDir, { recursive: true, force: true });
    }
  });

  test("preserves a bounded log tail before deleting a failed fixture", async () => {
    const ownedDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-failed-backend-"));
    const failure = new Error("backend exited");
    const output = vi.spyOn(console, "error").mockImplementation(() => {});
    const child = Object.assign(new EventEmitter(), {
      waitForLogFile: async () => {
        fs.writeFileSync(
          path.join(ownedDir, "backend-process.log"),
          "x".repeat(20_000) + "bind failed",
        );
      },
    }) as unknown as ChildProcess;
    try {
      await assert.rejects(
        runOwnedBackendFixture(
          ownedDir,
          async (register) => {
            register(child);
            throw failure;
          },
          { stopProcess: async () => {} },
        ),
        (error) => error === failure,
      );
      assert.equal(fs.existsSync(ownedDir), false);
      const text = output.mock.calls.flat().join(" ");
      assert.match(text, /bind failed/);
      assert.ok(text.length < 9_000);
    } finally {
      output.mockRestore();
      fs.rmSync(ownedDir, { recursive: true, force: true });
    }
  });

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
    const child = Object.assign(new EventEmitter(), {
      waitForLogFile: async () => {
        events.push("logs");
      },
    }) as unknown as ChildProcess;

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

    assert.deepEqual(events, ["run", "stop", "logs", "remove"]);
  });

  test("holds an early log error until the stream closes before removing the root", async () => {
    const events: string[] = [];
    const stream = new EventEmitter();
    const logError = new Error("log stream failed");
    const waitForLogFile = observeLogFile(stream);
    stream.once("close", () => events.push("close"));
    const child = Object.assign(new EventEmitter(), { waitForLogFile }) as unknown as ChildProcess;
    const unhandledRejections: unknown[] = [];
    const onUnhandledRejection = (reason: unknown) => unhandledRejections.push(reason);

    process.on("unhandledRejection", onUnhandledRejection);
    try {
      await assert.rejects(
        runOwnedBackendFixture(
          "/tmp/exact-owned-root",
          async (registerProcess) => {
            registerProcess(child);
            stream.emit("error", logError);
          },
          {
            stopProcess: async () => {
              events.push("stop");
              stream.emit("close");
            },
            removeTempRoot: () => events.push("remove"),
          },
        ),
        (error) => error === logError,
      );
      await new Promise<void>((resolve) => setImmediate(resolve));
      assert.deepEqual(unhandledRejections, []);
      assert.deepEqual(events, ["stop", "close", "remove"]);
    } finally {
      process.off("unhandledRejection", onUnhandledRejection);
    }
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

  test("returns the SSH bind mount to the host owner before removing the container", () => {
    const workDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-ssh-cleanup-test-"));
    const owner = fs.statSync(workDir);
    const commands: string[][] = [];
    const handle = {
      containerName: "kandev-sshd-e2e-7",
      workDir,
    } as SSHServerHandle;

    try {
      stopSSHServer(handle, (args) => {
        commands.push(args);
      });
    } finally {
      fs.rmSync(workDir, { recursive: true, force: true });
    }

    assert.deepEqual(commands, [
      [
        "exec",
        handle.containerName,
        "chown",
        "-R",
        `${owner.uid}:${owner.gid}`,
        "/home/kandev/.kandev",
      ],
      ["rm", "-f", handle.containerName],
    ]);
  });
});
