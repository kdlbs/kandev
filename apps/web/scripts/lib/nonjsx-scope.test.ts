/**
 * The scanned scope of `check-nonjsx-copy.mjs`.
 *
 * This exists because the check's failure mode is silent. It defaulted to
 * `i18nGuardFiles` for a long time and reported "✓ no non-JSX copy — 1421
 * guarded file(s) checked" the whole time, while 1141 source files were judged
 * by neither it nor the eslint rule. A clean run and a shrunken scope read the
 * same; only the denominator tells them apart, so the denominator is asserted.
 */
import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { i18nGuardFiles } from "../../eslint.i18n.options.mjs";
import { collectScannedFiles, isExcluded } from "./nonjsx-scope.mjs";

const WEB_ROOT = path.resolve(import.meta.dirname, "..", "..");

// `fs.globSync` landed in Node 22 and this repo pins @types/node 20, so the
// types do not know about it yet. The runtime (Node 24) does; the scripts have
// used it since they were written.
type GlobFs = { globSync: (pattern: string, options: { cwd: string }) => Iterable<string> };
const globFs = fs as unknown as GlobFs;

const scanned = () => collectScannedFiles(globFs, WEB_ROOT);

describe("isExcluded", () => {
  it("keeps ordinary source anywhere in the tree", () => {
    for (const file of [
      "lib/utils.ts",
      "components/task/task-cleanup-summary.ts",
      "hooks/domains/office/use-agent-route.ts",
      "app/stats/stats-utils.ts",
      "src/spa-routes.tsx",
      "vite.config.ts",
    ]) {
      expect(isExcluded(file)).toBe(false);
    }
  });

  it("drops tooling, build output, the catalog itself, and test support", () => {
    for (const file of [
      "node_modules/pkg/index.ts",
      "dist/bundle.ts",
      "e2e/tests/i18n/pseudo-coverage.spec.ts",
      "scripts/check-nonjsx-copy.mjs.ts",
      path.join("src", "locales", "en.ts"),
      "lib/thing.test.ts",
      "components/thing.test.tsx",
      "lib/ws/handlers/tasks.test-helpers.ts",
      "lib/state/dockview-panel-actions.test-utils.ts",
      "global.d.ts",
    ]) {
      expect(isExcluded(file)).toBe(true);
    }
  });

  it("does not drop a file merely because a path segment starts with an excluded name", () => {
    // `dist` vs `distribution`, `e2e` vs `e2e-helpers`: prefix matching without
    // the separator would silently unguard a real directory.
    expect(isExcluded("lib/distribution/plan.ts")).toBe(false);
    expect(isExcluded("components/scriptsy/panel.tsx")).toBe(false);
  });
});

describe("the default scope is the whole source tree", () => {
  it("scans thousands of files, not the guard allowlist", () => {
    const files = scanned();
    // The allowlist matched ~1420 files when this check was scoped to it. The
    // assertion is deliberately a floor well above that: it fails if anyone
    // narrows the scope back toward the allowlist, and does not churn as the
    // tree grows.
    expect(files.size).toBeGreaterThan(2000);
  });

  it("covers files that are not on the guard allowlist", () => {
    const allowlisted = new Set<string>();
    for (const entry of i18nGuardFiles) {
      for (const match of globFs.globSync(entry, { cwd: WEB_ROOT })) allowlisted.add(match);
    }
    const files = scanned();
    const offAllowlist = [...files].filter((file) => !allowlisted.has(file));

    expect(offAllowlist.length).toBeGreaterThan(500);
  });

  it("excludes e2e, scripts and the locale catalogs from the default scan", () => {
    const files = [...scanned()];

    expect(files.some((file) => file.startsWith(`e2e${path.sep}`))).toBe(false);
    expect(files.some((file) => file.startsWith(`scripts${path.sep}`))).toBe(false);
    expect(files.some((file) => file.includes(path.join("src", "locales")))).toBe(false);
    expect(files.some((file) => /\.test\.tsx?$/.test(file))).toBe(false);
  });

  it("still honours explicit globs, so the work-list use keeps working", () => {
    const files = collectScannedFiles(globFs, WEB_ROOT, ["lib/utils/*.ts"]);

    expect(files.size).toBeGreaterThan(0);
    expect([...files].every((file) => file.startsWith(path.join("lib", "utils")))).toBe(true);
  });
});
