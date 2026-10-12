import { describe, it, expect, vi, beforeEach } from "vitest";

const request = vi.fn();
const ensureMessageType = "session.ensure";
const launchMessageType = "session.launch";

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request }),
}));

import { ensureTaskSession, launchSession } from "./session-launch-service";

describe("ensureTaskSession", () => {
  beforeEach(() => {
    request.mockReset();
    request.mockResolvedValue({ success: true, task_id: "t1", state: "CREATED" });
  });

  it("sends the task id without auto_start when the option is absent", async () => {
    await ensureTaskSession("t1");
    expect(request).toHaveBeenCalledWith(ensureMessageType, { task_id: "t1" }, 60_000);
  });

  it("includes auto_start: false when explicitly requested", async () => {
    await ensureTaskSession("t1", { autoStart: false });
    expect(request).toHaveBeenCalledWith(
      ensureMessageType,
      { task_id: "t1", auto_start: false },
      60_000,
    );
  });

  it("includes auto_start: true when explicitly requested", async () => {
    await ensureTaskSession("t1", { autoStart: true });
    expect(request).toHaveBeenCalledWith(
      ensureMessageType,
      { task_id: "t1", auto_start: true },
      60_000,
    );
  });

  it("uses an explicit timeout when provided", async () => {
    await ensureTaskSession("t1", { timeout: 90_000 });
    expect(request).toHaveBeenCalledWith(ensureMessageType, { task_id: "t1" }, 90_000);
  });
});

describe("launchSession", () => {
  beforeEach(() => {
    request.mockReset();
    request.mockResolvedValue({ success: true, task_id: "t1", state: "CREATED" });
  });

  it("allows the default start request to finish native detection", async () => {
    const launchRequest = { task_id: "t1", intent: "start" as const };
    await launchSession(launchRequest);
    expect(request).toHaveBeenCalledWith(launchMessageType, launchRequest, 60_000);
  });

  it("allows the default resume request to finish native detection", async () => {
    const launchRequest = { task_id: "t1", intent: "resume" as const };
    await launchSession(launchRequest);
    expect(request).toHaveBeenCalledWith(launchMessageType, launchRequest, 60_000);
  });

  it("preserves a caller-provided timeout", async () => {
    const launchRequest = { task_id: "t1", intent: "start" as const };
    await launchSession(launchRequest, 90_000);
    expect(request).toHaveBeenCalledWith(launchMessageType, launchRequest, 90_000);
  });
});
