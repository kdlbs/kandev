import fs from "node:fs";

import { looksLikeManagedUnit } from "./templates";

export type WriteOutcome = "created" | "updated" | "unchanged" | "replaced-foreign";

/**
 * Write `content` to `targetPath` with idempotent + foreign-file handling.
 *
 * - Missing file → created (no warning)
 * - Existing managed file, same content → unchanged (no write, no warning)
 * - Existing managed file, different content → updated (overwrite, brief log)
 * - Existing file that doesn't look managed → replaced-foreign (overwrite,
 *   loud warning so the user notices we clobbered something)
 *
 * The "managed" check looks for kandev's marker substring; users who hand-edit
 * past the marker lose the no-op shortcut but still get an "updated" log line,
 * which is the expected workflow.
 */
export function writeUnitFile(targetPath: string, content: string): WriteOutcome {
  if (!fs.existsSync(targetPath)) {
    fs.writeFileSync(targetPath, content, { mode: 0o644 });
    console.log(`[kandev] wrote ${targetPath}`);
    return "created";
  }

  const existing = fs.readFileSync(targetPath, "utf8");
  if (existing === content) {
    console.log(`[kandev] ${targetPath} is already up to date`);
    return "unchanged";
  }

  if (!looksLikeManagedUnit(existing)) {
    console.log(
      `[kandev] WARNING: ${targetPath} exists but doesn't look like a kandev-managed file.`,
    );
    console.log(`[kandev]   The existing file will be replaced. A backup is saved alongside.`);
    fs.copyFileSync(targetPath, `${targetPath}.bak`);
    fs.writeFileSync(targetPath, content, { mode: 0o644 });
    console.log(`[kandev] replaced ${targetPath} (backup: ${targetPath}.bak)`);
    return "replaced-foreign";
  }

  fs.writeFileSync(targetPath, content, { mode: 0o644 });
  console.log(`[kandev] updated ${targetPath}`);
  return "updated";
}
