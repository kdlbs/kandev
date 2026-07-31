import { describe, expect, it } from "vitest";
import { connectionIssueDetails, connectionStatusDetails } from "./connection-status-item";

describe("connectionStatusDetails", () => {
  it.each([
    ["connected", "Connected", "Connected to Kandev"],
    ["connecting", "Connecting", "Connecting to Kandev"],
    ["reconnecting", "Reconnecting", "Reconnecting to Kandev"],
    ["disconnected", "Offline", "Connection unavailable"],
    ["error", "Connection error", "Connection error: socket closed"],
  ] as const)("maps %s to accessible connection details", (status, label, description) => {
    expect(
      connectionStatusDetails(status, status === "error" ? "socket closed" : null),
    ).toMatchObject({
      label,
      description,
    });
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
    expect(connectionIssueDetails(severity)).toMatchObject({ label, description, dotClass });
  });
});
