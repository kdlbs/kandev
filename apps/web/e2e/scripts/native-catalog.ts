import { execFileSync } from "node:child_process";
import fs from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

export type CatalogCase = { project: string; file: string; testId: string };
type CatalogIdentity = { sha: string; runId: string; runAttempt: string };
const nativeCache = new Map<string, CatalogCase[]>();

function identityKey(item: { project: string; file: string; testId?: string }): string {
  if (!item.testId) throw new Error("Missing native test ID in catalog coverage");
  return JSON.stringify([item.project, item.file, item.testId]);
}

function validateCases(value: unknown): CatalogCase[] {
  if (!Array.isArray(value) || value.length === 0) throw new Error("Missing native catalog cases");
  const seen = new Set<string>();
  for (const item of value as CatalogCase[]) {
    if (
      !item ||
      !["chromium", "mobile-chrome", "containers"].includes(item.project) ||
      typeof item.file !== "string" ||
      !item.file.startsWith("tests/") ||
      item.file.includes("\\") ||
      item.file.split("/").some((part) => !part || part === ".." || part === ".") ||
      typeof item.testId !== "string" ||
      !item.testId
    )
      throw new Error("Invalid native catalog case");
    const key = identityKey(item);
    if (seen.has(key)) throw new Error(`Duplicate native catalog case: ${key}`);
    seen.add(key);
  }
  return value as CatalogCase[];
}

export function discoverNativeCases(webRoot: string): CatalogCase[] {
  const root = path.resolve(webRoot);
  const cached = nativeCache.get(root);
  if (cached) return cached;
  const cli = createRequire(import.meta.url).resolve("@playwright/test/cli");
  const reporter = path.join(
    path.dirname(fileURLToPath(import.meta.url)),
    "native-catalog-reporter.ts",
  );
  const output = execFileSync(
    process.execPath,
    [
      cli,
      "test",
      "--config",
      path.join(root, "e2e/playwright.config.ts"),
      "--project=chromium",
      "--project=mobile-chrome",
      "--project=containers",
      "--list",
      `--reporter=${reporter}`,
    ],
    {
      cwd: root,
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    },
  );
  const report = JSON.parse(output) as { cases?: unknown; errors?: unknown };
  if (!Array.isArray(report.errors) || report.errors.length)
    throw new Error("Native catalog collection failed");
  const cases = validateCases(report.cases);
  nativeCache.set(root, cases);
  return cases;
}

export function githubCatalogIdentity(env = process.env): CatalogIdentity {
  const { GITHUB_SHA: sha, GITHUB_RUN_ID: runId, GITHUB_RUN_ATTEMPT: runAttempt } = env;
  if (
    !sha ||
    !/^[a-f0-9]{40}$/.test(sha) ||
    !runId ||
    !/^\d+$/.test(runId) ||
    !runAttempt ||
    !/^[1-9]\d*$/.test(runAttempt)
  ) {
    throw new Error("Missing GitHub native catalog identity");
  }
  return { sha, runId, runAttempt };
}

function verifyCheckout(webRoot: string, source: CatalogIdentity): void {
  const git = (...args: string[]) =>
    execFileSync("git", args, {
      cwd: webRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    }).trim();
  if (git("rev-parse", "HEAD") !== source.sha || git("diff", "--name-only", "HEAD", "--")) {
    throw new Error("Native catalog requires its unchanged checkout");
  }
  const inputs = [
    "e2e/tests",
    "e2e/fixtures",
    "e2e/helpers",
    "e2e/scripts",
    "e2e/playwright.config.ts",
  ];
  const untracked = git("ls-files", "--others", "--exclude-standard", "--", ...inputs);
  const ignored = git("ls-files", "--others", "--ignored", "--exclude-standard", "--", ...inputs);
  if (untracked || ignored)
    throw new Error("Native catalog checkout has uncommitted collection inputs");
}

export function writeCatalogSnapshot(file: string, webRoot: string): void {
  const source = githubCatalogIdentity();
  verifyCheckout(webRoot, source);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(
    file,
    `${JSON.stringify({ version: 1, source, cases: discoverNativeCases(webRoot) })}\n`,
  );
}

export function readCatalogSnapshot(
  file: string,
  webRoot: string,
  expected = githubCatalogIdentity({
    ...process.env,
    GITHUB_RUN_ATTEMPT: process.env.KANDEV_E2E_CATALOG_PRODUCER_ATTEMPT,
  }),
): CatalogCase[] {
  const snapshot = JSON.parse(fs.readFileSync(file, "utf8")) as {
    version?: number;
    source?: CatalogIdentity;
    cases?: unknown;
  };
  if (
    snapshot.version !== 1 ||
    !snapshot.source ||
    Object.entries(expected).some(
      ([key, value]) => snapshot.source![key as keyof CatalogIdentity] !== value,
    )
  ) {
    throw new Error("Native catalog identity does not match this workflow attempt");
  }
  verifyCheckout(webRoot, expected);
  return validateCases(snapshot.cases);
}

export function assertCatalogCoverage(
  catalog: CatalogCase[],
  actual: Array<{ project: string; file: string; testId?: string; retry?: number }>,
): void {
  const expected = new Set(validateCases(catalog).map(identityKey));
  const seen = new Map<string, Set<number>>();
  for (const test of actual) {
    const key = identityKey(test);
    if (!expected.has(key)) throw new Error(`Extra native test in report: ${key}`);
    const retry = test.retry ?? 0;
    if (!Number.isInteger(retry) || retry < 0) throw new Error(`Invalid native attempt: ${key}`);
    const attempts = seen.get(key) ?? new Set<number>();
    if (attempts.has(retry)) throw new Error(`Duplicate native test attempt in report: ${key}`);
    attempts.add(retry);
    seen.set(key, attempts);
  }
  const missing = [...expected].filter((key) => !seen.has(key));
  if (missing.length) throw new Error(`Missing native tests in report: ${missing.join(", ")}`);
  for (const [key, attempts] of seen) {
    if (!attempts.has(0) || Math.max(...attempts) !== attempts.size - 1) {
      throw new Error(`Missing native attempt history: ${key}`);
    }
  }
}
