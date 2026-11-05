import type {
  SandboxReportDecision,
  SandboxReportEvent,
  SandboxReportKind,
} from "./api/domains/sandbox-report-types";

export function filterReportEvents(
  events: SandboxReportEvent[],
  kind: SandboxReportKind | "all",
  decision: SandboxReportDecision | "all",
) {
  return events.filter(
    (event) =>
      (kind === "all" || event.kind === kind) &&
      (decision === "all" || event.decision === decision),
  );
}
export function reportEventDetails(event: SandboxReportEvent): string[] {
  const value = event.operation;
  if (value.filesystem) return [value.filesystem.access, value.filesystem.path];
  if (value.tcp)
    return [
      value.tcp.transport,
      value.tcp.host,
      value.tcp.port ? String(value.tcp.port) : "",
    ].filter(Boolean);
  if (value.command) return [value.command.id, value.command.caller];
  if (value.http_endpoint)
    return [
      value.http_endpoint.method,
      value.http_endpoint.origin,
      value.http_endpoint.path,
      value.http_endpoint.route_id,
    ].filter(Boolean);
  return [];
}
export function reportLearningIds(events: SandboxReportEvent[]) {
  return events
    .filter(
      (event) =>
        event.kind === "filesystem" &&
        Boolean(
          event.operation.filesystem &&
          ["read", "write", "read_write"].includes(event.operation.filesystem.access),
        ) &&
        event.exact_target &&
        !event.learning_blocker &&
        (event.decision === "denied" || event.decision === "approval_denied") &&
        event.legacy_record_id,
    )
    .map((event) => event.legacy_record_id!);
}
