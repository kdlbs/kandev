import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import {
  connectionIssueDetails,
  connectionStatusDetails,
  useConnectionIssueCopy,
} from "./connection-status-item";

/**
 * These helpers return catalog keys rather than copy (they are plain functions
 * with no hook available), so the assertions resolve through `t` — that covers
 * the key AND its catalog entry, which is what a reader actually gets.
 */
describe("connectionStatusDetails", () => {
  it.each([
    ["connected", "Connected", "Connected to Kandev"],
    ["connecting", "Connecting", "Connecting to Kandev"],
    ["reconnecting", "Reconnecting", "Reconnecting to Kandev"],
    ["disconnected", "Offline", "Connection unavailable"],
    ["error", "Connection error", "Connection error: socket closed"],
  ] as const)("maps %s to accessible connection details", (status, label, description) => {
    const details = connectionStatusDetails(status, status === "error" ? "socket closed" : null);

    expect(t(details.labelKey)).toBe(label);
    expect(t(details.descriptionKey, { ...details.descriptionValues })).toBe(description);
  });

  it("falls back to the undetailed message when the transport reports no error", () => {
    const details = connectionStatusDetails("error", null);

    expect(details.descriptionValues).toBeUndefined();
    expect(t(details.descriptionKey)).toBe("Connection error");
  });

  it("uses a small green dot for an active connection", () => {
    expect(connectionStatusDetails("connected", null)).toMatchObject({
      dotClass: "bg-success",
      animate: false,
    });
  });

  it.each([
    [
      "unstable",
      "Connection unstable",
      "Connection unstable. Reconnecting to Kandev.",
      "bg-amber-500",
    ],
    [
      "lost",
      "Connection lost",
      "Connection lost for at least 10 seconds. Live updates may be stale.",
      "bg-destructive",
    ],
  ] as const)("maps %s to a problem-only warning", (severity, label, description, dotClass) => {
    const details = connectionIssueDetails(severity);

    expect(t(details.labelKey)).toBe(label);
    expect(t(details.descriptionKey)).toBe(description);
    expect(details.dotClass).toBe(dotClass);
  });
});

describe("useConnectionIssueCopy", () => {
  it("returns nothing while the connection is healthy", () => {
    const { result } = renderHook(() => useConnectionIssueCopy("none"));

    expect(result.current).toBeNull();
  });

  it("resolves label and description for an active issue", () => {
    const { result } = renderHook(() => useConnectionIssueCopy("lost"));

    expect(result.current).toMatchObject({
      label: "Connection lost",
      description: "Connection lost for at least 10 seconds. Live updates may be stale.",
      dotClass: "bg-destructive",
    });
  });
});
