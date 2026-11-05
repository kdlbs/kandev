import { describe, expect, it } from "vitest";
import { filterReportEvents, reportEventDetails, reportLearningIds } from "./sandbox-report-events";
import type { SandboxReportEvent } from "./api/domains/sandbox-report-types";

const event: SandboxReportEvent = {
  id: "event",
  source_id: "source",
  provider_record_id: "request",
  sequence: 1,
  kind: "network",
  decision: "denied",
  reason_code: "permission_denied",
  occurred_at: "2026-11-02T12:00:00Z",
  exact_target: false,
  learning_blocker: "incomplete_identity",
  operation: { tcp: { host: "", port: 443, transport: "tcp" } },
};
describe("typed sandbox reports", () => {
  it("combines kind and decision filters without losing incomplete records", () => {
    const allowed = { ...event, id: "allowed", decision: "allowed" as const };
    expect(filterReportEvents([event, allowed], "network", "denied")).toEqual([event]);
    expect(filterReportEvents([event], "command", "all")).toEqual([]);
    expect(filterReportEvents([event], "all", "all")).toEqual([event]);
  });
  it("retains known partial fields without inventing a hostname or command arguments", () => {
    expect(reportEventDetails(event)).toEqual(["tcp", "443"]);
    expect(
      reportEventDetails({
        ...event,
        kind: "command",
        operation: { command: { id: "deploy", caller: "agent" } },
      }),
    ).toEqual(["deploy", "agent"]);
    expect(
      reportEventDetails({
        ...event,
        kind: "http_endpoint",
        operation: {
          http_endpoint: { route_id: "", origin: "", method: "GET", path: "/documents" },
        },
      }),
    ).toEqual(["GET", "/documents"]);
  });
  it("selects only exact denied records with a legacy bridge and no learning blocker", () => {
    const valid = {
      ...event,
      id: "valid",
      kind: "filesystem" as const,
      legacy_record_id: "legacy",
      exact_target: true,
      learning_blocker: "" as const,
      operation: { filesystem: { path: "/work/file", access: "read" } },
    };
    expect(
      reportLearningIds([
        event,
        valid,
        {
          ...valid,
          id: "read-write",
          legacy_record_id: "legacy-read-write",
          operation: { filesystem: { path: "/work/file", access: "read_write" } },
        },
        { ...valid, id: "allowed", decision: "allowed" },
        {
          ...valid,
          id: "foreign-kind",
          kind: "command",
          operation: { command: { id: "deploy", caller: "agent" } },
        },
      ]),
    ).toEqual(["legacy", "legacy-read-write"]);
  });
});
