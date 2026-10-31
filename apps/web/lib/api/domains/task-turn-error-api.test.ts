import { afterEach, describe, expect, it, vi } from "vitest";
import { dismissTaskTurnError } from "./task-turn-error-api";

const fetchJsonMock = vi.hoisted(() => vi.fn());

vi.mock("../client", () => ({ fetchJson: fetchJsonMock }));

afterEach(() => {
  vi.clearAllMocks();
});

describe("dismissTaskTurnError", () => {
  it("posts the observed stamp to the task-scoped endpoint", async () => {
    fetchJsonMock.mockResolvedValue({ dismissed: true });
    await dismissTaskTurnError("task-1", "failure-1");
    expect(fetchJsonMock).toHaveBeenCalledWith("/api/v1/tasks/task-1/turn-error/dismiss", {
      init: { method: "POST", body: JSON.stringify({ stamp: "failure-1" }) },
    });
  });

  it("lets the caller add request options without dropping the body", async () => {
    fetchJsonMock.mockResolvedValue({ dismissed: true });
    await dismissTaskTurnError("task-2", "failure-2", { init: { headers: { "X-Test": "1" } } });
    expect(fetchJsonMock).toHaveBeenCalledWith("/api/v1/tasks/task-2/turn-error/dismiss", {
      init: {
        headers: { "X-Test": "1" },
        method: "POST",
        body: JSON.stringify({ stamp: "failure-2" }),
      },
    });
  });
});
