import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useMarkdownImage } from "./use-markdown-image";
const client = vi.hoisted(() => ({ request: vi.fn() }));
const sessionA = "session-a";
const sessionB = "session-b";
const chartPath = "/tmp/chart.png";
const fileGetAction = "workspace.file.get";
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => client }));
beforeEach(() => {
  client.request.mockReset();
});
describe("session-local Markdown images", () => {
  it("reads the original absolute path through workspace.file.get", async () => {
    client.request.mockResolvedValue({ content: "aW1hZ2U=", is_binary: true });
    const { result } = renderHook(() => useMarkdownImage(sessionA, chartPath));
    await waitFor(() => expect(result.current.src).toBe("data:image/png;base64,aW1hZ2U="));
    expect(client.request).toHaveBeenCalledWith(fileGetAction, {
      session_id: sessionA,
      path: chartPath,
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
    const { result, rerender } = renderHook(({ session }) => useMarkdownImage(session, chartPath), {
      initialProps: { session: "old" },
    });
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
    rerender({ session: sessionA });
    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.src).toBeUndefined();
  });

  it("exposes unsupported image MIME types as an unavailable state", async () => {
    const { result } = renderHook(() => useMarkdownImage(sessionA, "/tmp/chart.tiff"));

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(client.request).not.toHaveBeenCalled();
  });

  it("shares a pending read between consumers with the same client, session, and path", async () => {
    let finish!: (value: unknown) => void;
    client.request.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const first = renderHook(() => useMarkdownImage(sessionA, chartPath));
    const second = renderHook(() => useMarkdownImage(sessionA, chartPath));

    expect(client.request).toHaveBeenCalledTimes(1);
    await act(async () => finish({ content: "aW1hZ2U=", is_binary: true }));

    await waitFor(() => {
      expect(first.result.current.src).toBe("data:image/png;base64,aW1hZ2U=");
      expect(second.result.current.src).toBe("data:image/png;base64,aW1hZ2U=");
    });
  });

  it("does not share a pending read across sessions", async () => {
    const finish: Array<(value: unknown) => void> = [];
    client.request.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish.push(resolve);
        }),
    );
    renderHook(() => useMarkdownImage(sessionA, chartPath));
    renderHook(() => useMarkdownImage(sessionB, chartPath));

    expect(client.request).toHaveBeenCalledTimes(2);
    expect(client.request).toHaveBeenNthCalledWith(1, fileGetAction, {
      session_id: sessionA,
      path: chartPath,
    });
    expect(client.request).toHaveBeenNthCalledWith(2, fileGetAction, {
      session_id: sessionB,
      path: chartPath,
    });
    await act(async () => {
      finish.forEach((resolve) => resolve({ content: "aW1hZ2U=", is_binary: true }));
    });
  });
});
