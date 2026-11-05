import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { SandboxDenial, SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import { useSandboxDenialVisibility } from "./use-sandbox-denial-visibility";

const denial: SandboxDenial = {
  id: "read-1",
  source_id: "audit",
  provider_record_id: "one",
  kind: "filesystem",
  operation: "read",
  target: "/workspace/denied",
  occurred_at: "2026-10-26T20:00:00Z",
};
const report = {
  attempt_id: "attempt",
  session_id: "session",
  policy_digest: "policy",
  ceiling_digest: "ceiling",
  provider: {
    installation_id: "installation",
    key: "provider",
    package_digest: "package",
    runtime_digest: "runtime",
  },
  data: { records: [denial, { ...denial, id: "write-1", operation: "write" }] },
} as SandboxReport;

describe("sandbox denial visibility", () => {
  it("hides only the selected operation without changing evidence and restores all records", () => {
    const before = structuredClone(report);
    const { result } = renderHook(() => useSandboxDenialVisibility(report));
    act(() => result.current.hide([denial.id]));
    expect(result.current.records.map((record) => record.id)).toEqual(["write-1"]);
    expect(result.current.hiddenCount).toBe(1);
    expect(report).toEqual(before);
    act(() => result.current.showAll());
    expect(result.current.records).toEqual(report.data.records);
    expect(result.current.hiddenCount).toBe(0);
  });

  it("keeps repeated matching notices hidden across refreshed captures", () => {
    const { result, rerender } = renderHook(({ value }) => useSandboxDenialVisibility(value), {
      initialProps: { value: report },
    });
    act(() => result.current.hide([denial.id]));
    rerender({
      value: {
        ...report,
        digest: "new-capture",
        data: {
          ...report.data,
          records: [
            { ...denial, id: "read-2" },
            denial,
            { ...denial, id: "elsewhere", target: "/different" },
          ],
        },
      },
    });
    expect(result.current.records.map((record) => record.id)).toEqual(["elsewhere"]);
    expect(result.current.hiddenCount).toBe(2);
  });

  it.each(["attempt_id", "session_id", "policy_digest", "ceiling_digest"] as const)(
    "does not hide denials in a changed %s",
    (field) => {
      const { result, rerender } = renderHook(({ value }) => useSandboxDenialVisibility(value), {
        initialProps: { value: report },
      });
      act(() => result.current.hide([denial.id]));
      rerender({ value: { ...report, [field]: "replacement" } });
      expect(result.current.records).toEqual(report.data.records);
      expect(result.current.hiddenCount).toBe(0);
    },
  );

  it("ignores unknown record IDs and isolates a replacement provider", () => {
    const { result, rerender } = renderHook(({ value }) => useSandboxDenialVisibility(value), {
      initialProps: { value: report },
    });
    act(() => result.current.hide(["missing"]));
    expect(result.current.hiddenCount).toBe(0);
    act(() => result.current.hide([denial.id]));
    rerender({
      value: { ...report, provider: { ...report.provider, installation_id: "replacement" } },
    });
    expect(result.current.hiddenCount).toBe(0);
  });
});
