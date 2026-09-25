import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = path.dirname(fileURLToPath(import.meta.url));
const APP_ROOT = path.join(here, "../../..");

/**
 * The left columns of the routine, trigger and run mapping tables in
 * docs/specs/office/system-design/routine-wire-contract.md, restricted to
 * keys containing an underscore. `id`, `name`, `status`, `kind`, `timezone`,
 * `enabled`, `variables` and `source` are deliberately excluded: they are
 * legitimate model-shape spellings that appear throughout these files, so
 * including them would fail correct code.
 */
const WIRE_KEYS = [
  "workspace_id",
  "task_template",
  "assignee_agent_profile_id",
  "concurrency_policy",
  "catch_up_policy",
  "catch_up_max",
  "last_run_at",
  "created_at",
  "updated_at",
  "routine_id",
  "cron_expression",
  "public_id",
  "signing_mode",
  "next_run_at",
  "last_fired_at",
  "trigger_id",
  "trigger_payload",
  "linked_task_id",
  "coalesced_into_run_id",
  "dispatch_fingerprint",
  "catch_up_missed_ticks",
  "catch_up_first_missed_at",
  "catch_up_truncated",
  "started_at",
  "completed_at",
  "skip_reason",
  "pause_id",
  "causation_id",
];

const WIRE_KEY_PATTERN = new RegExp(`\\b(${WIRE_KEYS.join("|")})\\b`);
const AS_UNKNOWN_AS_PATTERN = /\bas unknown as\b/;

function listSourceFiles(dir: string): string[] {
  const entries = readdirSync(dir);
  const files: string[] = [];
  for (const entry of entries) {
    const full = path.join(dir, entry);
    const stat = statSync(full);
    if (stat.isDirectory()) {
      files.push(...listSourceFiles(full));
      continue;
    }
    if (/\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry)) {
      files.push(full);
    }
  }
  return files;
}

function isExemptLine(line: string): boolean {
  const trimmed = line.trimStart();
  return trimmed.startsWith("//") || trimmed.startsWith("*");
}

function findMatches(files: string[], pattern: RegExp): string[] {
  const hits: string[] = [];
  for (const file of files) {
    const lines = readFileSync(file, "utf8").split("\n");
    lines.forEach((line, index) => {
      if (isExemptLine(line)) return;
      if (pattern.test(line)) {
        hits.push(`${path.relative(APP_ROOT, file)}:${index + 1}: ${line.trim()}`);
      }
    });
  }
  return hits;
}

const SCANNED_FILES = [
  ...listSourceFiles(path.join(APP_ROOT, "app/office/routines")),
  path.join(APP_ROOT, "src/office-routine-client-routes.tsx"),
];

describe("routine wire contract scan (AC-OFFICE-ROUTINE-WIRE-003.10)", () => {
  it("contains no snake_case wire key outside comments and tests", () => {
    expect(findMatches(SCANNED_FILES, WIRE_KEY_PATTERN)).toEqual([]);
  });

  it("contains no `as unknown as` cast", () => {
    expect(findMatches(SCANNED_FILES, AS_UNKNOWN_AS_PATTERN)).toEqual([]);
  });
});

describe("s.office.routines consumer set (AC-OFFICE-ROUTINE-WIRE-003.18)", () => {
  it("is exactly the three known files under apps/web/app/**", () => {
    const appFiles = listSourceFiles(path.join(APP_ROOT, "app"));
    const consumers = appFiles
      .filter((file) => {
        const lines = readFileSync(file, "utf8").split("\n");
        return lines.some((line) => !isExemptLine(line) && line.includes("s.office.routines"));
      })
      .map((file) => path.relative(APP_ROOT, file))
      .sort();

    expect(consumers).toEqual(
      [
        "app/office/agents/[id]/layout.tsx",
        "app/office/agents/[id]/runs/runs-list-view.tsx",
        "app/office/routines/routines-content.tsx",
      ].sort(),
    );
  });
});
