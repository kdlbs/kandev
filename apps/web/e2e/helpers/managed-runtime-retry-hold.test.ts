import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, it, vi } from "vitest";

it.skipIf(process.platform === "win32").each(["transient", "silent-exit"])(
  "holds the %s replacement until the browser releases startup",
  async (mode) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), "managed-retry-hold-"));
    const release = path.join(directory, "release");
    const started = path.join(directory, "started");
    const mock = path.join(directory, "mock-agent");
    fs.writeFileSync(mock, '#!/bin/sh\nprintf "%s" "$*" > "$E2E_TEST_STARTED"\n', {
      mode: 0o755,
    });
    const env = {
      ...process.env,
      NPM_CONFIG_CACHE: directory,
      E2E_NPX_MODE: mode,
      E2E_NPX_LAUNCH_ID: "held-retry",
      E2E_NPX_RETRY_RELEASE_FILE: release,
      E2E_MOCK_AGENT_PATH: mock,
      E2E_TEST_STARTED: started,
    };
    const args = [
      path.resolve("e2e/fixtures/managed-runtime-npx.sh"),
      "--yes",
      "--prefer-offline",
      "opencode-ai@1.0.0",
      "acp",
    ];
    expect(spawnSync("sh", args, { env }).status).toBe(1);
    const child = spawn("sh", args, { env });
    const closed = once(child, "close");
    try {
      await vi.waitFor(() => expect(fs.existsSync(`${release}.waiting`)).toBe(true));
      expect(fs.existsSync(started)).toBe(false);
      fs.writeFileSync(release, "released");
      expect((await closed)[0]).toBe(0);
      expect(fs.readFileSync(started, "utf8")).toBe("acp");
      expect(fs.existsSync(`${release}.waiting`)).toBe(false);
    } finally {
      fs.writeFileSync(release, "released");
      await closed;
      fs.rmSync(directory, { recursive: true, force: true });
    }
  },
);
