"use client";

import { snapshotLogs } from "@/lib/logger/buffer";
import {
  uploadFrontendLog,
  type ImproveKandevBootstrapResponse,
} from "@/lib/api/domains/improve-kandev-api";

/**
 * Append the bundle file paths to the user-supplied description as a
 * machine-readable footer the agent prompt instructs to read.
 *
 * Behavior:
 * - When `bootstrap` is null, returns the description unchanged.
 * - When `captureLogs` is false, returns the description unchanged.
 * - When `captureLogs` is true, attempts to upload the current in-memory
 *   frontend log snapshot to the bundle directory and only includes the
 *   `frontend_log` entry when the upload succeeded — referencing a file
 *   that was never written would mislead the agent.
 */
export async function buildImproveKandevDescription(
  description: string,
  bootstrap: ImproveKandevBootstrapResponse | null,
  captureLogs: boolean,
): Promise<string> {
  if (!bootstrap) return description;
  if (!captureLogs) return description;

  let frontendLogUploaded = false;
  try {
    await uploadFrontendLog(bootstrap.bundle_dir, snapshotLogs());
    frontendLogUploaded = true;
  } catch {
    // Frontend log upload is best-effort — keep submitting the task without it.
  }

  const lines = [
    description,
    "",
    "---",
    "Context bundle for the agent:",
    `- ${bootstrap.bundle_files.metadata}`,
    `- ${bootstrap.bundle_files.backend_log}`,
  ];
  if (frontendLogUploaded) {
    lines.push(`- ${bootstrap.bundle_files.frontend_log}`);
  }
  return lines.join("\n");
}
