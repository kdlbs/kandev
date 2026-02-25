import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { findBundleRoot, resolveWebServerPath } from "./bundle";

describe("findBundleRoot", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-bundle-test-"));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("returns kandev/ subdir when it exists", () => {
    const kandevDir = path.join(tmpDir, "kandev");
    fs.mkdirSync(kandevDir);
    expect(findBundleRoot(tmpDir)).toBe(kandevDir);
  });

  it("returns cacheDir itself when kandev/ does not exist", () => {
    expect(findBundleRoot(tmpDir)).toBe(tmpDir);
  });
});

describe("resolveWebServerPath", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-web-test-"));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("finds web/server.js (non-monorepo)", () => {
    const serverPath = path.join(tmpDir, "web", "server.js");
    fs.mkdirSync(path.dirname(serverPath), { recursive: true });
    fs.writeFileSync(serverPath, "");
    expect(resolveWebServerPath(tmpDir)).toBe(serverPath);
  });

  it("finds web/web/server.js (monorepo, apps root)", () => {
    const serverPath = path.join(tmpDir, "web", "web", "server.js");
    fs.mkdirSync(path.dirname(serverPath), { recursive: true });
    fs.writeFileSync(serverPath, "");
    expect(resolveWebServerPath(tmpDir)).toBe(serverPath);
  });

  it("finds web/apps/web/server.js (monorepo, project root)", () => {
    const serverPath = path.join(tmpDir, "web", "apps", "web", "server.js");
    fs.mkdirSync(path.dirname(serverPath), { recursive: true });
    fs.writeFileSync(serverPath, "");
    expect(resolveWebServerPath(tmpDir)).toBe(serverPath);
  });

  it("returns null when no server.js found", () => {
    expect(resolveWebServerPath(tmpDir)).toBeNull();
  });

  it("prefers first match when multiple exist", () => {
    const first = path.join(tmpDir, "web", "server.js");
    const second = path.join(tmpDir, "web", "web", "server.js");
    fs.mkdirSync(path.dirname(first), { recursive: true });
    fs.writeFileSync(first, "");
    fs.mkdirSync(path.dirname(second), { recursive: true });
    fs.writeFileSync(second, "");
    expect(resolveWebServerPath(tmpDir)).toBe(first);
  });
});
