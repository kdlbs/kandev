#!/usr/bin/env tsx
/**
 * Pre-build / on-demand script that compiles a single JSON manifest of every
 * third-party OSS dependency (npm + Go) shipped with kandev, along with its
 * resolved license. The output is committed and read statically by the future
 * `/settings/system/licenses` page — zero runtime cost, no network access.
 *
 * Output: apps/web/generated/licenses.json
 *
 * npm side  -> `pnpm licenses list --json --prod` over the workspace.
 * Go side   -> `go-licenses report ./...` from `apps/backend/` with a custom
 *              template so we can read each module's local LICENSE file.
 *
 * Run with: `pnpm --filter @kandev/web licenses:gen`
 */
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

type Ecosystem = "npm" | "go";

interface LicenseEntry {
  name: string;
  version: string;
  license: string;
  repository?: string;
  license_text?: string;
  stale?: boolean;
  ecosystem: Ecosystem;
}

const __dirname = dirname(fileURLToPath(import.meta.url));
const WEB_ROOT = join(__dirname, "..");
const APPS_ROOT = join(WEB_ROOT, "..");
const REPO_ROOT = join(APPS_ROOT, "..");
const BACKEND_ROOT = join(REPO_ROOT, "apps", "backend");
const OUTPUT_DIR = join(WEB_ROOT, "generated");
const OUTPUT_FILE = join(OUTPUT_DIR, "licenses.json");

const MAX_LICENSE_TEXT_BYTES = 64 * 1024; // 64 KiB guard

// SPDX-ish identifiers that mean "we don't know" — normalize them.
const UNKNOWN_LICENSE = "UNKNOWN";

function readSafe(path: string): string | undefined {
  try {
    const buf = readFileSync(path);
    if (buf.length > MAX_LICENSE_TEXT_BYTES) {
      return buf.subarray(0, MAX_LICENSE_TEXT_BYTES).toString("utf-8");
    }
    return buf.toString("utf-8");
  } catch {
    return undefined;
  }
}

function normalizeLicense(raw: string | undefined): string {
  if (!raw) return UNKNOWN_LICENSE;
  const trimmed = raw.trim();
  if (!trimmed || trimmed.toLowerCase() === "unknown") return UNKNOWN_LICENSE;
  return trimmed;
}

function repoFromPackageJson(pkgPath: string): string | undefined {
  const data = readSafe(join(pkgPath, "package.json"));
  if (!data) return undefined;
  try {
    const json = JSON.parse(data) as {
      repository?: string | { url?: string };
      homepage?: string;
    };
    const repo = json.repository;
    if (typeof repo === "string") return repo;
    if (repo && typeof repo.url === "string") {
      return repo.url.replace(/^git\+/, "").replace(/\.git$/, "");
    }
    return json.homepage;
  } catch {
    return undefined;
  }
}

function findLicenseFile(pkgPath: string): string | undefined {
  const candidates = [
    "LICENSE",
    "LICENSE.md",
    "LICENSE.txt",
    "LICENSE-MIT",
    "LICENSE-APACHE",
    "License",
    "license",
    "license.md",
    "license.txt",
    "COPYING",
    "COPYING.txt",
  ];
  for (const name of candidates) {
    const path = join(pkgPath, name);
    if (existsSync(path)) return path;
  }
  return undefined;
}

function readLicenseText(pkgPath: string | undefined): string | undefined {
  if (!pkgPath) return undefined;
  const licensePath = findLicenseFile(pkgPath);
  if (!licensePath) return undefined;
  return readSafe(licensePath);
}

interface PnpmPackage {
  name: string;
  versions?: string[];
  version?: string;
  paths?: string[];
  path?: string;
  license?: string;
  homepage?: string;
}

type PnpmLicensesOutput = Record<string, PnpmPackage[]>;

function collectNpmEntries(): LicenseEntry[] {
  let raw: string;
  try {
    raw = execFileSync("pnpm", ["licenses", "list", "--json", "--prod"], {
      cwd: APPS_ROOT,
      encoding: "utf-8",
      maxBuffer: 128 * 1024 * 1024,
      stdio: ["ignore", "pipe", "inherit"],
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    process.stderr.write(
      `[licenses] warning: failed to enumerate npm dependencies via pnpm: ${message}\n`,
    );
    return [];
  }

  let parsed: PnpmLicensesOutput;
  try {
    parsed = JSON.parse(raw) as PnpmLicensesOutput;
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    process.stderr.write(`[licenses] warning: invalid JSON from pnpm licenses: ${message}\n`);
    return [];
  }

  const entries: LicenseEntry[] = [];
  for (const [licenseId, pkgs] of Object.entries(parsed)) {
    for (const pkg of pkgs) {
      entries.push(...flattenPnpmPackage(pkg, licenseId));
    }
  }
  return entries;
}

function flattenPnpmPackage(pkg: PnpmPackage, fallbackLicense: string): LicenseEntry[] {
  const versions = pkg.versions ?? (pkg.version ? [pkg.version] : []);
  const paths = pkg.paths ?? (pkg.path ? [pkg.path] : []);
  const license = normalizeLicense(pkg.license ?? fallbackLicense);
  // pnpm collapses multiple versions of the same package into one record,
  // but the path array is aligned by index — pair them.
  const results: LicenseEntry[] = [];
  for (let i = 0; i < versions.length; i++) {
    const version = versions[i];
    const pkgPath = paths[i] ?? paths[0];
    const repo = pkgPath ? repoFromPackageJson(pkgPath) : undefined;
    const licenseText = readLicenseText(pkgPath);
    results.push({
      name: pkg.name,
      version,
      license,
      ...(repo ? { repository: repo } : {}),
      ...(licenseText ? { license_text: licenseText } : {}),
      ecosystem: "npm",
    });
  }
  return results;
}

interface GoRow {
  pkg: string;
  licensePath: string;
  licenseURL: string;
  licenseName: string;
}

function collectGoEntries(): LicenseEntry[] {
  const probe = spawnSync("go-licenses", ["--help"], { stdio: "ignore" });
  if (probe.error || probe.status !== 0) {
    process.stderr.write(
      "[licenses] error: `go-licenses` not found on PATH.\n" +
        "  Install it with:  go install github.com/google/go-licenses@latest\n" +
        "  Then re-run:      pnpm --filter @kandev/web licenses:gen\n",
    );
    throw new Error("go-licenses is required");
  }

  const template = `{{range .}}{{.Name}}|||{{with .LicensePath}}{{.}}{{end}}|||{{with .LicenseURL}}{{.}}{{end}}|||{{with .LicenseName}}{{.}}{{end}}
{{end}}`;
  const tmplPath = join(tmpdir(), `kandev-go-licenses-${process.pid}.tpl`);
  writeFileSync(tmplPath, template);

  let raw: string;
  try {
    raw = execFileSync("go-licenses", ["report", "./...", "--template", tmplPath], {
      cwd: BACKEND_ROOT,
      encoding: "utf-8",
      maxBuffer: 64 * 1024 * 1024,
      // Discard noisy "Failed to find license" stderr — those are kandev's own
      // packages, not third-party. The CSV body still contains every resolved dep.
      stdio: ["ignore", "pipe", "pipe"],
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    process.stderr.write(`[licenses] warning: go-licenses failed: ${message}\n`);
    const recovery = recoverGoEntriesAfterReportFailure(execStdout(err), readExistingManifest());
    if (recovery?.source === "go-licenses-stdout") {
      process.stderr.write(
        `[licenses] warning: recovered ${recovery.entries.length} Go entries from go-licenses stdout.\n`,
      );
      return recovery.entries;
    }
    if (recovery?.source === "existing-manifest") {
      process.stderr.write(
        `[licenses] warning: reusing ${recovery.entries.length} stale Go entries from the existing manifest.\n`,
      );
      return recovery.entries;
    }
    throw new Error("go-licenses failed and no existing Go entries are available");
  } finally {
    rmSync(tmplPath, { force: true });
  }

  return goEntriesFromReport(raw);
}

type GoEntryRecoverySource = "go-licenses-stdout" | "existing-manifest";

interface GoEntryRecovery {
  entries: LicenseEntry[];
  source: GoEntryRecoverySource;
}

export function recoverGoEntriesAfterReportFailure(
  stdout: string,
  existingManifestRaw?: string,
): GoEntryRecovery | null {
  const recoveredEntries = goEntriesFromReport(stdout);
  if (recoveredEntries.length > 0) {
    return { entries: recoveredEntries, source: "go-licenses-stdout" };
  }
  const existingEntries = existingManifestRaw
    ? markStaleGoEntries(readGoEntriesFromManifest(existingManifestRaw))
    : [];
  if (existingEntries.length > 0) {
    return { entries: existingEntries, source: "existing-manifest" };
  }
  return null;
}

function goEntriesFromReport(raw: string): LicenseEntry[] {
  return parseGoRows(raw).filter(isThirdPartyGoRow).map(toGoEntry);
}

function execStdout(err: unknown): string {
  const stdout = (err as { stdout?: Buffer | string }).stdout;
  if (typeof stdout === "string") return stdout;
  if (Buffer.isBuffer(stdout)) return stdout.toString("utf-8");
  return "";
}

function readExistingManifest(): string | undefined {
  try {
    return readFileSync(OUTPUT_FILE, "utf-8");
  } catch {
    return undefined;
  }
}

export function readGoEntriesFromManifest(raw: string): LicenseEntry[] {
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isLicenseEntry).filter((entry) => entry.ecosystem === "go");
  } catch {
    return [];
  }
}

function isLicenseEntry(value: unknown): value is LicenseEntry {
  if (!value || typeof value !== "object") return false;
  const entry = value as Partial<Record<keyof LicenseEntry, unknown>>;
  return (
    typeof entry.name === "string" &&
    typeof entry.version === "string" &&
    typeof entry.license === "string" &&
    (entry.ecosystem === "npm" || entry.ecosystem === "go") &&
    (entry.repository === undefined || typeof entry.repository === "string") &&
    (entry.license_text === undefined || typeof entry.license_text === "string") &&
    (entry.stale === undefined || typeof entry.stale === "boolean")
  );
}

function markStaleGoEntries(entries: LicenseEntry[]): LicenseEntry[] {
  return entries.map((entry) => ({ ...entry, stale: true }));
}

function parseGoRows(raw: string): GoRow[] {
  const rows: GoRow[] = [];
  for (const line of raw.split(/\r?\n/)) {
    if (!line.trim()) continue;
    const [pkg, licensePath, licenseURL, licenseName] = line.split("|||");
    if (!pkg) continue;
    rows.push({
      pkg: pkg.trim(),
      licensePath: (licensePath ?? "").trim(),
      licenseURL: (licenseURL ?? "").trim(),
      licenseName: (licenseName ?? "").trim(),
    });
  }
  return rows;
}

function isThirdPartyGoRow(row: GoRow): boolean {
  return !row.pkg.startsWith("github.com/kandev/kandev");
}

// Extract module name + version from go-licenses' `LicenseURL` / `LicensePath`.
// We try a few common URL shapes (GitHub blob, cs.opensource.google, generic
// `.../@version/...` in the gopath cache) and fall back to the package path.
function moduleAndVersion(row: GoRow): { module: string; version: string } {
  // GitHub-style: https://<host>/<owner>/<repo>/blob/<version>/<path>
  const github = row.licenseURL.match(/^https?:\/\/([^/]+)\/([^/]+)\/([^/]+)\/blob\/([^/]+)\//);
  if (github) {
    const [, host, owner, repo, version] = github;
    return { module: `${host}/${owner}/${repo}`, version };
  }
  // cs.opensource.google-style: https://cs.opensource.google/go/x/<repo>/+/<version>:LICENSE
  const csgoogle = row.licenseURL.match(/cs\.opensource\.google\/(.+?)\/\+\/([^:/]+)/);
  if (csgoogle) {
    return { module: `golang.org/${csgoogle[1]}`, version: csgoogle[2] };
  }
  // Fall back to the local gopath cache path: .../<module>@<version>/LICENSE
  const cachePath = row.licensePath.match(/[\\/]([^\\/]+)@([^\\/]+)[\\/]/);
  if (cachePath) {
    // The first capture is the leaf dir; reconstruct the module from row.pkg
    // because the leaf alone loses the host/owner prefix.
    return { module: stripVersionSuffix(row.pkg), version: cachePath[2] };
  }
  return { module: row.pkg, version: "" };
}

// Some modules embed a major version in their import path (`/v2`, `/v3`, etc).
// Keep that suffix — it's part of the canonical module name in Go.
function stripVersionSuffix(pkg: string): string {
  return pkg;
}

function repoFromGoURL(licenseURL: string): string | undefined {
  const github = licenseURL.match(/^(https?:\/\/[^/]+\/[^/]+\/[^/]+)\/blob\//);
  if (github) return github[1];
  const csgoogle = licenseURL.match(/^(https?:\/\/cs\.opensource\.google\/[^/]+\/[^/]+)\//);
  if (csgoogle) return csgoogle[1];
  return undefined;
}

function toGoEntry(row: GoRow): LicenseEntry {
  const { module, version } = moduleAndVersion(row);
  const licenseText = row.licensePath ? readSafe(row.licensePath) : undefined;
  const repo = repoFromGoURL(row.licenseURL);
  return {
    name: module,
    version: version || "unknown",
    license: normalizeLicense(row.licenseName),
    ...(repo ? { repository: repo } : {}),
    ...(licenseText ? { license_text: licenseText } : {}),
    ecosystem: "go",
  };
}

function dedupe(entries: LicenseEntry[]): LicenseEntry[] {
  const seen = new Map<string, LicenseEntry>();
  for (const e of entries) {
    const key = `${e.ecosystem}|${e.name}|${e.version}`;
    const existing = seen.get(key);
    if (!existing) {
      seen.set(key, e);
      continue;
    }
    // Prefer the entry with the richer payload (license_text, repository).
    seen.set(key, mergeEntries(existing, e));
  }
  return [...seen.values()];
}

function mergeEntries(a: LicenseEntry, b: LicenseEntry): LicenseEntry {
  return {
    name: a.name,
    version: a.version,
    license: a.license !== UNKNOWN_LICENSE ? a.license : b.license,
    repository: a.repository ?? b.repository,
    license_text: a.license_text ?? b.license_text,
    ...(a.stale || b.stale ? { stale: true } : {}),
    ecosystem: a.ecosystem,
  };
}

function sortEntries(entries: LicenseEntry[]): LicenseEntry[] {
  return [...entries].sort((a, b) => {
    if (a.ecosystem !== b.ecosystem) return a.ecosystem < b.ecosystem ? -1 : 1;
    if (a.name !== b.name) return a.name < b.name ? -1 : 1;
    if (a.version !== b.version) return a.version < b.version ? -1 : 1;
    return 0;
  });
}

function main(): void {
  mkdirSync(OUTPUT_DIR, { recursive: true });

  process.stderr.write("[licenses] collecting npm entries via pnpm…\n");
  const npm = collectNpmEntries();
  process.stderr.write(`[licenses]   npm entries: ${npm.length}\n`);

  let goEntries: LicenseEntry[] = [];
  try {
    process.stderr.write("[licenses] collecting go entries via go-licenses…\n");
    goEntries = collectGoEntries();
    process.stderr.write(`[licenses]   go entries: ${goEntries.length}\n`);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    process.stderr.write(`[licenses] go ingestion failed: ${message}\n`);
    process.exit(1);
  }

  const merged = sortEntries(dedupe([...npm, ...goEntries]));
  const json = JSON.stringify(merged, null, 2) + "\n";
  writeFileSync(OUTPUT_FILE, json);
  process.stderr.write(`[licenses] wrote ${merged.length} entries -> ${OUTPUT_FILE}\n`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main();
}
