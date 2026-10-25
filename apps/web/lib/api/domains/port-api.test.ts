import { beforeEach, describe, expect, it, vi } from "vitest";
import { listTunnels } from "./port-api";

const { getClientMock, requestMock } = vi.hoisted(() => ({
  getClientMock: vi.fn(),
  requestMock: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: getClientMock }));

beforeEach(() => {
  requestMock.mockReset();
  getClientMock.mockReset().mockReturnValue({ request: requestMock });
});

describe("tunnel snapshot reads", () => {
  it("distinguishes an unavailable connection from an empty tunnel list", async () => {
    getClientMock.mockReturnValue(null);
    expect(await listTunnels("session")).toBeNull();
  });

  it("distinguishes a failed request from an empty tunnel list", async () => {
    requestMock.mockRejectedValue(new Error("connection lost"));
    expect(await listTunnels("session")).toBeNull();
  });

  it("returns a successful empty tunnel snapshot", async () => {
    requestMock.mockResolvedValue({ tunnels: [] });
    expect(await listTunnels("session")).toEqual([]);
  });

  it("returns the current tunnel snapshot", async () => {
    const tunnels = [{ port: 9000, tunnel_port: 49152 }];
    requestMock.mockResolvedValue({ tunnels });
    expect(await listTunnels("session")).toEqual(tunnels);
    expect(requestMock).toHaveBeenCalledWith("port.tunnel.list", { session_id: "session" });
  });
});
