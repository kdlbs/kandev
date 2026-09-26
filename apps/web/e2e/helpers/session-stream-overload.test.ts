import { describe, expect, it, vi } from "vitest";
import { noisyReceivedFrames } from "./session-stream-overload";
import type { GatewayTrafficFrame } from "./ws-traffic";

vi.mock("@playwright/test", () => ({ expect: {} }));

function messageFrame(
  action: "session.message.added" | "session.message.updated",
  sessionId: string,
): GatewayTrafficFrame {
  return {
    direction: "received",
    action,
    sessionId,
    bytes: 1,
    timestamp: 1,
  };
}

describe("noisyReceivedFrames", () => {
  it("includes added and updated message frames for the selected session", () => {
    const added = messageFrame("session.message.added", "noisy");
    const updated = messageFrame("session.message.updated", "noisy");
    const otherSession = messageFrame("session.message.added", "quiet");

    expect(noisyReceivedFrames([added, updated, otherSession], "noisy")).toEqual([added, updated]);
  });
});
