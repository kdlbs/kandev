import { describe, it, expect, vi, beforeEach } from "vitest";

const requestMock = vi.fn();
let clientFactory: () => { request: typeof requestMock } | null = () => ({ request: requestMock });

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => clientFactory(),
}));

// Imported after the mock so it picks up the mocked module.
const { renameSession } = await import("./session-api");

beforeEach(() => {
  requestMock.mockReset();
  requestMock.mockResolvedValue({ success: true });
  clientFactory = () => ({ request: requestMock });
});

describe("renameSession", () => {
  it("sends session.rename with the session id and name", async () => {
    await renameSession("sess-1", "reviewer");
    expect(requestMock).toHaveBeenCalledWith("session.rename", {
      session_id: "sess-1",
      name: "reviewer",
    });
  });

  it("passes an empty name through so the custom label can be cleared", async () => {
    await renameSession("sess-1", "");
    expect(requestMock).toHaveBeenCalledWith("session.rename", { session_id: "sess-1", name: "" });
  });

  it("throws when the WebSocket client is unavailable", async () => {
    clientFactory = () => null;
    await expect(renameSession("sess-1", "reviewer")).rejects.toThrow("WebSocket unavailable");
    expect(requestMock).not.toHaveBeenCalled();
  });

  it("propagates backend errors to the caller", async () => {
    requestMock.mockRejectedValueOnce(new Error("session not found"));
    await expect(renameSession("missing", "x")).rejects.toThrow("session not found");
  });
});
