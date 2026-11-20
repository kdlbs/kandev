export function summaryHasRun(payload: unknown, runId: string, status?: string): boolean {
  if (!payload || typeof payload !== "object" || !("summaries" in payload)) return false;
  if (!Array.isArray(payload.summaries)) return false;
  return payload.summaries.some((item: unknown) => {
    if (!item || typeof item !== "object" || !("last_run" in item)) return false;
    const run = item.last_run;
    if (!run || typeof run !== "object" || !("id" in run) || run.id !== runId) return false;
    return status === undefined || ("status" in run && run.status === status);
  });
}
