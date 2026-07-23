import { describe, expect, it } from "vitest";
import { connectionStatusDetails } from "./connection-status-item";

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
});
