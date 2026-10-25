import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  prepareCompactRuntimeFixture,
  prepareCompactRuntimeRun,
} from "../e2e/fixtures/compact-runtime";

let root: string;
let sourceBinDir: string;
let teardown: (() => void) | undefined;
const identity = { version: "0.0.0-e2e.shared", commit: "b".repeat(40) };

beforeEach(() => {
  root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-compact-run-test-"));
  vi.stubEnv("TMPDIR", root);
  sourceBinDir = path.join(root, "bin");
  fs.mkdirSync(sourceBinDir);
  fs.writeFileSync(
    path.join(sourceBinDir, "e2e-build-identity.json"),
    JSON.stringify({
      schema_version: 1,
      source_revision: identity.commit,
    }),
  );
  for (const name of ["kandev", "agentctl", "agentctl-linux-amd64"]) {
    fs.writeFileSync(
      path.join(sourceBinDir, name),
      name === "kandev" ? `#!/bin/sh\nprintf '%s\\n' '${identity.version}'\n` : `${name} fixture\n`,
      { mode: 0o755 },
    );
  }
  vi.stubEnv("KANDEV_E2E_COMPACT_MANIFEST", "");
});

afterEach(() => {
  teardown?.();
  teardown = undefined;
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
  fs.rmSync(root, { recursive: true, force: true });
});

function worker(name: string, overrides = identity) {
  return prepareCompactRuntimeFixture({
    sourceBinDir,
    bundleDir: path.join(root, name, "bundle"),
    homeDir: path.join(root, name, "home"),
    identity: overrides,
  });
}

describe("compact runtime invocation", () => {
  it("builds one canonical manifest while keeping worker bundles and caches independent", () => {
    teardown = prepareCompactRuntimeRun({ sourceBinDir });
    const manifestPath = process.env.KANDEV_E2E_COMPACT_MANIFEST!;
    const first = worker("first");
    const second = worker("second");
    expect(fs.existsSync(path.join(root, "first", "remote-helpers.json"))).toBe(false);
    expect(fs.existsSync(path.join(root, "second", "remote-helpers.json"))).toBe(false);
    expect(fs.readFileSync(path.join(first.bundleDir, "remote-helpers.json"), "utf8")).toBe(
      fs.readFileSync(manifestPath, "utf8"),
    );
    fs.writeFileSync(first.cachePath, "changed first worker");
    expect(fs.readFileSync(second.cachePath, "utf8")).toBe("agentctl-linux-amd64 fixture\n");
    teardown();
    expect(fs.existsSync(path.dirname(manifestPath))).toBe(false);
    expect(process.env.KANDEV_E2E_COMPACT_MANIFEST).toBe("");
    expect(fs.existsSync(second.cachePath)).toBe(true);
  });

  it.each(["version", "commit"] as const)(
    "rejects a shared manifest from a different %s",
    (field) => {
      teardown = prepareCompactRuntimeRun({ sourceBinDir });
      expect(() =>
        worker("wrong-identity", { ...identity, [field]: `${identity[field]}different` }),
      ).toThrow(/manifest identity/i);
    },
  );

  it("rejects a helper changed after global preparation", () => {
    teardown = prepareCompactRuntimeRun({ sourceBinDir });
    fs.writeFileSync(path.join(sourceBinDir, "agentctl-linux-amd64"), "different helper");
    expect(() => worker("wrong-helper")).toThrow(/helper.*digest/i);
  });

  it("removes temporary inputs and preserves the environment when canonical generation fails", () => {
    fs.writeFileSync(
      path.join(sourceBinDir, "kandev"),
      "#!/bin/sh\nprintf '%s\\n' 'invalid version!'\n",
      { mode: 0o755 },
    );
    expect(() => prepareCompactRuntimeRun({ sourceBinDir })).toThrow();
    expect(fs.readdirSync(root)).toEqual(["bin"]);
    expect(process.env.KANDEV_E2E_COMPACT_MANIFEST).toBe("");
  });
});
