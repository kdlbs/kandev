/**
 * Pre-build script that parses CHANGELOG.md into a JSON array of version entries
 * for the changelog settings page to import at build time.
 *
 * Output: apps/web/generated/changelog.json
 * Format: [{ version, date, notes }, ...]
 */
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const WEB_ROOT = join(__dirname, "..");
const REPO_ROOT = join(WEB_ROOT, "..", "..");
const OUTPUT_DIR = join(WEB_ROOT, "generated");
const OUTPUT_FILE = join(OUTPUT_DIR, "changelog.json");
const CHANGELOG_PATH = join(REPO_ROOT, "CHANGELOG.md");

function parseChangelog(content) {
  const entries = [];
  // Match version headings: ## VERSION - DATE or ## VERSION
  const headerPattern = /^## (\S+?)(?:\s*-\s*(\S+))?\s*$/gm;

  let match;
  const headers = [];
  while ((match = headerPattern.exec(content)) !== null) {
    headers.push({
      version: match[1],
      date: match[2] || "",
      index: match.index,
      end: match.index + match[0].length,
    });
  }

  for (let i = 0; i < headers.length; i++) {
    const header = headers[i];
    const bodyStart = header.end;
    const bodyEnd = i + 1 < headers.length ? headers[i + 1].index : content.length;
    const notes = content.slice(bodyStart, bodyEnd).trim();

    if (header.version === "Unreleased") continue;

    entries.push({
      version: header.version,
      date: header.date,
      notes,
    });
  }

  return entries;
}

function main() {
  mkdirSync(OUTPUT_DIR, { recursive: true });

  let content;
  try {
    content = readFileSync(CHANGELOG_PATH, "utf-8");
  } catch {
    writeFileSync(OUTPUT_FILE, "[]\n");
    console.log("[changelog] CHANGELOG.md not found — wrote empty array");
    return;
  }

  const entries = parseChangelog(content);
  writeFileSync(OUTPUT_FILE, JSON.stringify(entries, null, 2) + "\n");
  console.log(`[changelog] parsed ${entries.length} version entries`);
}

main();
