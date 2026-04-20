import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { resolveStandaloneServerPath } from "./start";

function makeRepo(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), "kandev-start-test-"));
}

function writeFile(filePath: string, contents = "") {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, contents);
}

describe("resolveStandaloneServerPath", () => {
  let repoRoot: string;

  beforeEach(() => {
    repoRoot = makeRepo();
  });

  afterEach(() => {
    fs.rmSync(repoRoot, { recursive: true, force: true });
  });

  it("returns the expected path when .next/standalone/web/server.js exists", () => {
    const expected = path.join(repoRoot, "apps", "web", ".next", "standalone", "web", "server.js");
    writeFile(expected);

    expect(resolveStandaloneServerPath(repoRoot)).toBe(expected);
  });

  it("finds server.js at a deeper path (Turbopack root mismatch)", () => {
    const nested = path.join(
      repoRoot,
      "apps",
      "web",
      ".next",
      "standalone",
      "Users",
      "alice",
      "projects",
      "kandev",
      "apps",
      "web",
      "server.js",
    );
    writeFile(nested);

    expect(resolveStandaloneServerPath(repoRoot)).toBe(nested);
  });

  it("returns null when .next/standalone/ does not exist", () => {
    expect(resolveStandaloneServerPath(repoRoot)).toBeNull();
  });

  it("returns null when standalone/ exists but has no web/server.js", () => {
    const standaloneDir = path.join(repoRoot, "apps", "web", ".next", "standalone");
    fs.mkdirSync(standaloneDir, { recursive: true });
    // Stray server.js that is NOT inside a `web/` directory.
    writeFile(path.join(standaloneDir, "node_modules", "next", "server.js"));

    expect(resolveStandaloneServerPath(repoRoot)).toBeNull();
  });
});
