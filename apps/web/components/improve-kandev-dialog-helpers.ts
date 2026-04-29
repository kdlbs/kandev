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
 * If captureLogs is true, also upload the current in-memory frontend log
 * snapshot to the bundle directory before returning.
 */
export async function buildImproveKandevDescription(
  description: string,
  bootstrap: ImproveKandevBootstrapResponse | null,
  captureLogs: boolean,
): Promise<string> {
  if (!bootstrap) return description;
  if (!captureLogs) return description;

  try {
    await uploadFrontendLog(bootstrap.bundle_dir, snapshotLogs());
  } catch {
    // Frontend log upload is best-effort — surface failure to the user via
    // the surrounding try/catch in the dialog rather than aborting submission.
  }

  const lines = [
    description,
    "",
    "---",
    "Context bundle for the agent:",
    `- ${bootstrap.bundle_files.metadata}`,
    `- ${bootstrap.bundle_files.backend_log}`,
    `- ${bootstrap.bundle_files.frontend_log}`,
  ];
  return lines.join("\n");
}
