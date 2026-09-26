import { describe, it, expect, vi, beforeEach } from "vitest";

const request = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request }),
}));

import { ensureTaskSession, launchSession } from "./session-launch-service";

describe("launchSession conversation fork payload", () => {
  beforeEach(() => {
    request.mockReset();
    request.mockResolvedValue({ success: true, task_id: "t1", state: "RUNNING" });
  });

  it("sends the frozen fork and creation request ids with the new session", async () => {
    await launchSession({
      task_id: "t1",
      intent: "start",
      prompt: "Continue the work",
      conversation_fork_id: "fork-1",
      creation_request_id: "create-1",
    });

    expect(request).toHaveBeenCalledWith(
      "session.launch",
      expect.objectContaining({
        task_id: "t1",
        intent: "start",
        conversation_fork_id: "fork-1",
        creation_request_id: "create-1",
      }),
      15_000,
    );
  });
});

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
