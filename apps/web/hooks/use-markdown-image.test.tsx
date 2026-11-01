import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useMarkdownImage } from "./use-markdown-image";
const client = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => client }));
beforeEach(() => {
  client.request.mockReset();
});
describe("session-local Markdown images", () => {
  it("reads the original absolute path through workspace.file.get", async () => {
    client.request.mockResolvedValue({ content: "aW1hZ2U=", is_binary: true });
    const { result } = renderHook(() => useMarkdownImage("session-a", "/tmp/chart.png"));
    await waitFor(() => expect(result.current.src).toBe("data:image/png;base64,aW1hZ2U="));
    expect(client.request).toHaveBeenCalledWith("workspace.file.get", {
      session_id: "session-a",
      path: "/tmp/chart.png",
    });
  });
  it("ignores a late image response after switching sessions", async () => {
    let finish!: (v: unknown) => void;
    client.request.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    client.request.mockResolvedValueOnce({ content: "bmV3", is_binary: true });
    const { result, rerender } = renderHook(
      ({ session }) => useMarkdownImage(session, "/tmp/chart.png"),
      { initialProps: { session: "old" } },
    );
    rerender({ session: "new" });
    await waitFor(() => expect(result.current.src).toContain("bmV3"));
    await act(async () => finish({ content: "b2xk", is_binary: true }));
    expect(result.current.src).toContain("bmV3");
  });
  it("does not read without the originating session and exposes missing-file errors", async () => {
    const { result, rerender } = renderHook(
      ({ session }) => useMarkdownImage(session, "/tmp/missing.png"),
      { initialProps: { session: null as string | null } },
    );
    expect(client.request).not.toHaveBeenCalled();
    client.request.mockRejectedValue(new Error("not found"));
    rerender({ session: "session-a" });
    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.src).toBeUndefined();
  });
});
