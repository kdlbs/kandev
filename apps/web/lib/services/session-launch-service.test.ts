import { describe, it, expect, vi, beforeEach } from "vitest";

const request = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request }),
}));

import { ensureTaskSession } from "./session-launch-service";

describe("ensureTaskSession", () => {
  beforeEach(() => {
    request.mockReset();
    request.mockResolvedValue({ success: true, task_id: "t1", state: "CREATED" });
  });

  it("sends the task id without auto_start when the option is absent", async () => {
    await ensureTaskSession("t1");
    expect(request).toHaveBeenCalledWith("session.ensure", { task_id: "t1" }, 15_000);
  });

  it("includes auto_start: false when explicitly requested", async () => {
    await ensureTaskSession("t1", { autoStart: false });
    expect(request).toHaveBeenCalledWith(
      "session.ensure",
      { task_id: "t1", auto_start: false },
      15_000,
    );
  });

  it("includes auto_start: true when explicitly requested", async () => {
    await ensureTaskSession("t1", { autoStart: true });
    expect(request).toHaveBeenCalledWith(
      "session.ensure",
      { task_id: "t1", auto_start: true },
      15_000,
    );
  });
});
