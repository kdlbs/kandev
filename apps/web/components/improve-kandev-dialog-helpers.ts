"use client";

import {
  leaseDiagnosticBundle,
  type ImproveKandevBootstrapResponse,
} from "@/lib/api/domains/improve-kandev-api";
import { createDiagnosticBundle, fetchDiagnosticBundle } from "@/lib/api/domains/system-api";
import type { DiagnosticBundleJob } from "@/lib/types/system";

/**
 * Creates the same owner-scoped frontend+backend ZIP used by System Logs,
 * leases it into the Improve Kandev task context, and appends that single path
 * to the task description. Diagnostics stay best-effort.
 */
export async function buildImproveKandevDescription(
  description: string,
  bootstrap: ImproveKandevBootstrapResponse | null,
  captureLogs: boolean,
): Promise<string> {
  if (!bootstrap) return description;
  if (!captureLogs) return description;

  try {
    const created = await createDiagnosticBundle(["backend", "frontend"]);
    const completed = await waitForDiagnosticBundle(created);
    const lease = await leaseDiagnosticBundle(bootstrap.bundle_dir, completed.id);
    return [
      description,
      "",
      "---",
      "Diagnostic bundle for the agent (frontend + backend logs):",
      `- ${lease.path}`,
    ].join("\n");
  } catch {
    return description;
  }
}

async function waitForDiagnosticBundle(initial: DiagnosticBundleJob): Promise<DiagnosticBundleJob> {
  let job = initial;
  const deadline = Date.now() + 5 * 60_000;
  while (job.status === "collecting" || job.status === "building") {
    if (Date.now() >= deadline) throw new Error("diagnostic bundle timed out");
    await new Promise((resolve) => setTimeout(resolve, 250));
    job = await fetchDiagnosticBundle(job.id);
  }
  if (job.status !== "ready" && job.status !== "partial") {
    throw new Error("diagnostic bundle failed");
  }
  return job;
}
