/**
 * Pre-build script that extracts the current version's release notes
 * from CHANGELOG.md and writes them as JSON for the web app to import.
 *
 * Version resolution order:
 *   1. KANDEV_VERSION env var (set by CI)
 *   2. `git describe --tags --abbrev=0` (local dev with tags)
 *   3. "dev" fallback
 */
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { execSync } from "node:child_process";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const WEB_ROOT = join(__dirname, "..");
const REPO_ROOT = join(WEB_ROOT, "..", "..");
const OUTPUT_DIR = join(WEB_ROOT, "generated");
const OUTPUT_FILE = join(OUTPUT_DIR, "release-notes.json");
const CHANGELOG_PATH = join(REPO_ROOT, "CHANGELOG.md");

const FALLBACK = { version: "dev", date: "", notes: "" };

function getVersion() {
  if (process.env.KANDEV_VERSION) {
    return process.env.KANDEV_VERSION.replace(/^v/, "");
  }
  try {
    const tag = execSync("git describe --tags --abbrev=0", {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
    return tag.replace(/^v/, "");
  } catch {
    return "dev";
  }
}

function extractVersionNotes(changelog, version) {
  // Split on ## headings and find the section for our version
  const escapedVersion = version.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const headerPattern = new RegExp(`^## ${escapedVersion}(?:\\s*-\\s*(\\S+))?\\s*$`, "m");
  const headerMatch = changelog.match(headerPattern);
  if (!headerMatch) return null;

  const date = headerMatch[1] || "";
  const sectionStart = headerMatch.index + headerMatch[0].length;
  const rest = changelog.slice(sectionStart);

  // Find the next ## heading (or end of string)
  const nextHeading = rest.search(/^## /m);
  const body = nextHeading === -1 ? rest : rest.slice(0, nextHeading);
  const notes = body.trim();

  return { date, notes };
}

function findLatestVersionInChangelog(changelog) {
  const headerPattern = /^## (\d[\d.]+)(?:\s*-\s*(\S+))?\s*$/gm;
  let match;
  while ((match = headerPattern.exec(changelog)) !== null) {
    const version = match[1];
    const extracted = extractVersionNotes(changelog, version);
    if (extracted && extracted.notes) {
      return { version, ...extracted };
    }
  }
  return null;
}

function main() {
  mkdirSync(OUTPUT_DIR, { recursive: true });
  const version = getVersion();

  if (version === "dev") {
    writeFileSync(OUTPUT_FILE, JSON.stringify(FALLBACK, null, 2) + "\n");
    console.log("[release-notes] dev mode — wrote fallback");
    return;
  }

  let changelog;
  try {
    changelog = readFileSync(CHANGELOG_PATH, "utf-8");
  } catch {
    writeFileSync(OUTPUT_FILE, JSON.stringify(FALLBACK, null, 2) + "\n");
    console.log("[release-notes] CHANGELOG.md not found — wrote fallback");
    return;
  }

  const extracted = extractVersionNotes(changelog, version);
  if (!extracted || !extracted.notes) {
    // Current version has no changelog entry yet — fall back to the most recent released version
    const latest = findLatestVersionInChangelog(changelog);
    if (latest) {
      writeFileSync(OUTPUT_FILE, JSON.stringify({ version: latest.version, date: latest.date, notes: latest.notes }, null, 2) + "\n");
      console.log(`[release-notes] version ${version} not in CHANGELOG.md — fell back to ${latest.version}`);
    } else {
      writeFileSync(OUTPUT_FILE, JSON.stringify({ version, date: "", notes: "" }, null, 2) + "\n");
      console.log(`[release-notes] version ${version} not found in CHANGELOG.md — wrote empty notes`);
    }
    return;
  }

  const result = { version, date: extracted.date, notes: extracted.notes };
  writeFileSync(OUTPUT_FILE, JSON.stringify(result, null, 2) + "\n");
  console.log(`[release-notes] extracted notes for v${version}`);
}

main();
