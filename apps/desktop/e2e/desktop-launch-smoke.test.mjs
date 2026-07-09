import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
import { test } from "node:test";

import {
  HEALTH_REQUESTED_TIMEOUT_MS,
  ROOT_REQUESTED_TIMEOUT_MS,
  waitForFile,
} from "./desktop-launch-smoke.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const backendRsPath = resolve(__dirname, "../src-tauri/src/backend.rs");

async function withTempDir(run) {
  const dir = await mkdtemp(join(tmpdir(), "wait-for-file-"));
  try {
    return await run(dir);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

test("waitForFile resolves once the target file appears", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "marker");
    const write = new Promise((r) => setTimeout(r, 50)).then(() => writeFile(target, "1"));
    await Promise.all([waitForFile(target, 2_000), write]);
  });
});

test("waitForFile throws a timeout error including the describeDetail() text", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "never-appears");
    await assert.rejects(
      () => waitForFile(target, 100, undefined, () => "custom diagnostic detail"),
      (err) => {
        assert.match(err.message, /Timed out waiting for/);
        assert.match(err.message, /custom diagnostic detail/);
        return true;
      },
    );
  });
});

test("waitForFile omits the detail block when describeDetail is not provided", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "never-appears");
    await assert.rejects(
      () => waitForFile(target, 100),
      (err) => {
        assert.equal(err.message, `Timed out waiting for ${target}`);
        return true;
      },
    );
  });
});

test("waitForFile calls tick on every poll and surfaces a tick failure immediately", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "never-appears");
    let calls = 0;
    await assert.rejects(
      () =>
        waitForFile(target, 5_000, () => {
          calls += 1;
          if (calls >= 2) {
            throw new Error("boom");
          }
        }),
      /boom/,
    );
    assert.ok(calls >= 2, `expected at least 2 tick() calls, got ${calls}`);
  });
});

test("health-requested timeout stays above the Rust backend's own HEALTH_TIMEOUT", async () => {
  const source = await readFile(backendRsPath, "utf8");
  const match = source.match(/const HEALTH_TIMEOUT: Duration = Duration::from_secs\((\d+)\);/);
  assert.ok(
    match,
    "could not find HEALTH_TIMEOUT in backend.rs — update this test if it moved or was renamed",
  );

  const rustHealthTimeoutMs = Number(match[1]) * 1_000;
  assert.ok(
    HEALTH_REQUESTED_TIMEOUT_MS > rustHealthTimeoutMs,
    `HEALTH_REQUESTED_TIMEOUT_MS (${HEALTH_REQUESTED_TIMEOUT_MS}ms) must exceed backend.rs's HEALTH_TIMEOUT ` +
      `(${rustHealthTimeoutMs}ms) — otherwise this smoke test can kill a launcher the Rust side would still ` +
      "consider healthy-pending, turning a legitimately slow CI run into a spurious failure.",
  );
  assert.ok(
    ROOT_REQUESTED_TIMEOUT_MS > 0 && Number.isInteger(ROOT_REQUESTED_TIMEOUT_MS),
    "ROOT_REQUESTED_TIMEOUT_MS must be a positive integer",
  );
});
