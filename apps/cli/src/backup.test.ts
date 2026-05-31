import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { backupProductionDb, isProductionDb } from "./backup";

describe("isProductionDb", () => {
  it("returns false for dev-isolated paths under .kandev-dev", () => {
    expect(isProductionDb("/repo/.kandev-dev/data/kandev.db")).toBe(false);
    expect(isProductionDb("/.kandev-dev/kandev.db")).toBe(false);
  });

  it("returns true for production and custom paths", () => {
    expect(isProductionDb("/home/user/.kandev/data/kandev.db")).toBe(true);
    expect(isProductionDb("/tmp/custom.db")).toBe(true);
    expect(isProductionDb("/data/kandev.db")).toBe(true);
  });
});

describe("backupProductionDb", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-backup-test-"));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it("returns null when the db does not exist", () => {
    const result = backupProductionDb(path.join(tmpDir, "missing.db"));
    expect(result).toBeNull();
  });

  it("creates a backup and keeps only the 5 newest", () => {
    const dbPath = path.join(tmpDir, ".kandev", "data", "kandev.db");
    fs.mkdirSync(path.dirname(dbPath), { recursive: true });
    fs.writeFileSync(dbPath, "original-db-content");

    // Create 6 backups with explicit per-iteration timestamps so both the
    // filename clock and mtime advance deterministically - no spin loop, no
    // dependence on Date.now() resolution.
    for (let i = 0; i < 6; i++) {
      const stamp = new Date(1_700_000_000_000 + i * 1000);
      const result = backupProductionDb(dbPath, tmpDir, stamp);
      expect(result).not.toBeNull();
    }

    const backupDir = path.join(tmpDir, ".kandev", "data", "backups");
    const files = fs
      .readdirSync(backupDir)
      .filter((f) => f.startsWith("dev-prod-db-") && f.endsWith(".db"))
      .sort();

    expect(files.length).toBe(5);
  });
});
