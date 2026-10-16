import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  assertCatalogCoverage,
  discoverNativeCases,
  readCatalogSnapshot,
  writeCatalogSnapshot,
  type CatalogCase,
} from "./native-catalog";
import { hashSourceFile, parseBlobReports } from "./e2e-timings";
import { summarizeObservations } from "./retry-summary";
import { createShardPlan, NORMAL_SHARD_COUNT } from "./plan-shards";
import { readManifest } from "./run-planned-shard";

const directories: string[] = [];
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
  for (const directory of directories.splice(0))
    fs.rmSync(directory, { recursive: true, force: true });
});

function directory() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-catalog-"));
  directories.push(root);
  return root;
}

const cases: CatalogCase[] = [
  { project: "chromium", file: "tests/example.spec.ts", testId: "desktop-id" },
  { project: "mobile-chrome", file: "tests/example.spec.ts", testId: "phone-id" },
];

function snapshot() {
  const root = directory();
  const webRoot = path.join(root, "apps/web");
  fs.mkdirSync(path.join(webRoot, "e2e/tests"), { recursive: true });
  fs.writeFileSync(path.join(webRoot, "e2e/tests/example.spec.ts"), "// committed spec\n");
  const git = (...args: string[]) =>
    execFileSync("git", args, {
      cwd: root,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    }).trim();
  git("init");
  git("add", ".");
  git(
    "-c",
    "user.name=Catalog fixture",
    "-c",
    "user.email=fixture@example.invalid",
    "-c",
    "commit.gpgsign=false",
    "commit",
    "-m",
    "fixture",
  );
  const source = { sha: git("rev-parse", "HEAD"), runId: "123", runAttempt: "1" };
  const file = path.join(root, "native-catalog.json");
  const write = (value: unknown) => fs.writeFileSync(file, JSON.stringify(value));
  write({ version: 1, source, cases });
  return { root, webRoot, file, source, write };
}

describe("single-producer native catalog", () => {
  it("retains native IDs for every project, skipped/generated case and helper owner", () => {
    const webRoot = directory();
    fs.mkdirSync(path.join(webRoot, "e2e/tests"), { recursive: true });
    fs.symlinkSync(path.resolve("node_modules"), path.join(webRoot, "node_modules"), "dir");
    fs.writeFileSync(
      path.join(webRoot, "e2e/playwright.config.ts"),
      `export default { testDir: './tests', projects: [{ name: 'chromium' }, { name: 'mobile-chrome' }, { name: 'containers' }] };`,
    );
    fs.writeFileSync(
      path.join(webRoot, "e2e/tests/helper.ts"),
      `import { test } from '@playwright/test'; export function shared() { test.describe('helper group', () => test('helper case', async () => {})); }`,
    );
    fs.writeFileSync(
      path.join(webRoot, "e2e/tests/example.spec.ts"),
      `import { test } from '@playwright/test'; import { shared } from './helper'; shared(); test.skip('existing skip', async () => {}); for (const name of ['one', 'two']) test(name, async () => {});`,
    );

    const catalog = discoverNativeCases(webRoot);
    expect(catalog).toHaveLength(12);
    expect(new Set(catalog.map((item) => item.testId)).size).toBe(12);
    expect(new Set(catalog.map((item) => item.file))).toEqual(new Set(["tests/example.spec.ts"]));
    const cli = path.resolve("node_modules/@playwright/test/cli.js");
    const blobs = path.join(webRoot, "blobs");
    execFileSync(
      process.execPath,
      [cli, "test", "--config", "e2e/playwright.config.ts", "--workers=1", "--reporter=blob"],
      {
        cwd: webRoot,
        env: { ...process.env, PLAYWRIGHT_BLOB_OUTPUT_DIR: blobs },
        stdio: "pipe",
      },
    );
    const actual = summarizeObservations(parseBlobReports(blobs), "2026-10-15T00:00:00Z").tests;
    expect(actual).toHaveLength(12);
    expect(() => assertCatalogCoverage(catalog, actual)).not.toThrow();
    expect(() => assertCatalogCoverage(catalog, actual.slice(1))).toThrow(/Missing/);

    const git = (...args: string[]) =>
      execFileSync("git", args, { cwd: webRoot, encoding: "utf8", stdio: "pipe" }).trim();
    git("init");
    git("add", "e2e");
    git(
      "-c",
      "user.name=Catalog fixture",
      "-c",
      "user.email=fixture@example.invalid",
      "-c",
      "commit.gpgsign=false",
      "commit",
      "-m",
      "fixture",
    );
    vi.stubEnv("GITHUB_SHA", git("rev-parse", "HEAD"));
    vi.stubEnv("GITHUB_RUN_ID", "123");
    vi.stubEnv("GITHUB_RUN_ATTEMPT", "1");
    vi.stubEnv("KANDEV_E2E_CATALOG_PRODUCER_ATTEMPT", "1");
    const snapshotFile = path.join(webRoot, "native-catalog.json");
    writeCatalogSnapshot(snapshotFile, webRoot);
    const args = [
      path.resolve("node_modules/tsx/dist/cli.mjs"),
      path.resolve("e2e/scripts/retry-summary.ts"),
      "--input",
      blobs,
      "--output",
      path.join(webRoot, "summary.json"),
    ];
    const run = (withCatalog = true) =>
      execFileSync(process.execPath, withCatalog ? [...args, "--catalog", snapshotFile] : args, {
        cwd: webRoot,
        env: { ...process.env, GITHUB_ACTIONS: "true" },
        stdio: "pipe",
      });
    expect(() => run()).not.toThrow();
    const duplicate = path.join(blobs, "duplicate.zip");
    fs.copyFileSync(
      path.join(blobs, fs.readdirSync(blobs).find((file) => file.endsWith(".zip"))!),
      duplicate,
    );
    expect(() => run()).toThrow(/Duplicate native test attempt/);
    fs.unlinkSync(duplicate);
    expect(() => run(false)).toThrow(/Required CI native catalog/);
  }, 30_000);

  it("accepts the matching immutable catalog without rediscovering tests", () => {
    const fixture = snapshot();
    expect(readCatalogSnapshot(fixture.file, fixture.webRoot, fixture.source)).toEqual(cases);
  });

  it("runs the complete CI manifest validation without importing the suite again", () => {
    const fixture = snapshot();
    const catalog = cases.map((item) => ({
      project: item.project,
      file: item.file,
      fileHash: hashSourceFile(fixture.webRoot, item.file),
      testCount: 1,
    }));
    const manifest = createShardPlan(catalog, {
      cohort: "normal",
      shardCount: NORMAL_SHARD_COUNT,
      generatedAt: "2026-10-15T00:00:00Z",
    });
    const manifestRoot = path.join(fixture.root, "manifests");
    fs.mkdirSync(path.join(manifestRoot, "normal"), { recursive: true });
    fs.copyFileSync(fixture.file, path.join(manifestRoot, "native-catalog.json"));
    const file = path.join(manifestRoot, "normal/1.json");
    fs.writeFileSync(file, JSON.stringify(manifest));
    vi.spyOn(process, "cwd").mockReturnValue(fixture.webRoot);
    vi.stubEnv("GITHUB_ACTIONS", "true");
    vi.stubEnv("GITHUB_SHA", fixture.source.sha);
    vi.stubEnv("GITHUB_RUN_ID", fixture.source.runId);
    vi.stubEnv("GITHUB_RUN_ATTEMPT", fixture.source.runAttempt);
    vi.stubEnv("KANDEV_E2E_CATALOG_PRODUCER_ATTEMPT", fixture.source.runAttempt);
    expect(readManifest(file)).toEqual(manifest);
    fs.unlinkSync(path.join(manifestRoot, "native-catalog.json"));
    expect(() => readManifest(file)).toThrow();
  });

  it("binds a rerun consumer to the successful Build attempt", () => {
    const fixture = snapshot();
    vi.stubEnv("GITHUB_SHA", fixture.source.sha);
    vi.stubEnv("GITHUB_RUN_ID", fixture.source.runId);
    vi.stubEnv("GITHUB_RUN_ATTEMPT", "2");
    vi.stubEnv("KANDEV_E2E_CATALOG_PRODUCER_ATTEMPT", "1");
    expect(readCatalogSnapshot(fixture.file, fixture.webRoot)).toEqual(cases);
    vi.stubEnv("KANDEV_E2E_CATALOG_PRODUCER_ATTEMPT", "");
    expect(() => readCatalogSnapshot(fixture.file, fixture.webRoot)).toThrow(/identity/);
  });

  it.each(["sha", "runId", "runAttempt"] as const)("rejects a foreign %s", (key) => {
    const fixture = snapshot();
    fixture.write({ version: 1, source: { ...fixture.source, [key]: "foreign" }, cases });
    expect(() => readCatalogSnapshot(fixture.file, fixture.webRoot, fixture.source)).toThrow(
      /identity/,
    );
  });

  it.each(["changed spec", "changed helper", "added spec", "ignored spec"])(
    "rejects %s",
    (kind) => {
      const fixture = snapshot();
      if (kind === "changed spec")
        fs.appendFileSync(path.join(fixture.webRoot, "e2e/tests/example.spec.ts"), "// changed\n");
      if (kind === "changed helper")
        fs.writeFileSync(path.join(fixture.webRoot, "e2e/tests/helper.ts"), "// new helper\n");
      if (kind === "added spec")
        fs.writeFileSync(path.join(fixture.webRoot, "e2e/tests/added.spec.ts"), "// new test\n");
      if (kind === "ignored spec") {
        fs.writeFileSync(path.join(fixture.root, ".git/info/exclude"), "ignored.spec.ts\n");
        fs.writeFileSync(
          path.join(fixture.webRoot, "e2e/tests/ignored.spec.ts"),
          "// ignored test\n",
        );
      }
      expect(() => readCatalogSnapshot(fixture.file, fixture.webRoot, fixture.source)).toThrow(
        /checkout/,
      );
    },
  );

  it("rejects a mismatched checkout even when the artifact identity matches", () => {
    const fixture = snapshot();
    const source = { ...fixture.source, sha: "a".repeat(40) };
    fixture.write({ version: 1, source, cases });
    expect(() => readCatalogSnapshot(fixture.file, fixture.webRoot, source)).toThrow(/checkout/);
  });

  it.each(["missing", "malformed", "empty", "duplicate", "escaping file"])(
    "rejects %s catalog evidence",
    (kind) => {
      const fixture = snapshot();
      if (kind === "missing") fs.unlinkSync(fixture.file);
      if (kind === "malformed") fs.writeFileSync(fixture.file, "{");
      if (kind === "empty") fixture.write({ version: 1, source: fixture.source, cases: [] });
      if (kind === "duplicate")
        fixture.write({ version: 1, source: fixture.source, cases: [...cases, cases[0]] });
      if (kind === "escaping file")
        fixture.write({
          version: 1,
          source: fixture.source,
          cases: [{ ...cases[0], file: "../elsewhere.spec.ts" }],
        });
      expect(() => readCatalogSnapshot(fixture.file, fixture.webRoot, fixture.source)).toThrow();
    },
  );

  it.each(["missing", "extra", "duplicate", "wrong project", "wrong owner", "missing ID"])(
    "rejects %s actual coverage",
    (kind) => {
      const actual: Array<{ project: string; file: string; testId?: string }> = cases.map(
        (item) => ({
          ...item,
        }),
      );
      if (kind === "missing") actual.pop();
      if (kind === "extra") actual.push({ ...cases[0]!, testId: "extra-id" });
      if (kind === "duplicate") actual.push({ ...actual[0]! });
      if (kind === "wrong project") actual[0]!.project = "containers";
      if (kind === "wrong owner") actual[0]!.file = "tests/helper.ts";
      if (kind === "missing ID") delete actual[0]!.testId;
      expect(() => assertCatalogCoverage(cases, actual)).toThrow();
    },
  );

  it("retains retries while rejecting duplicated or incomplete native attempts", () => {
    const attempts = [
      { ...cases[0]!, retry: 0 },
      { ...cases[0]!, retry: 1 },
      { ...cases[1]!, retry: 0 },
    ];
    expect(() => assertCatalogCoverage(cases, attempts)).not.toThrow();
    expect(() => assertCatalogCoverage(cases, [...attempts, attempts[0]!])).toThrow(/Duplicate/);
    expect(() => assertCatalogCoverage(cases, attempts.slice(1))).toThrow(/attempt/);
  });
});
