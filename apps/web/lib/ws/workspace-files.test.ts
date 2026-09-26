import { describe, expect, it, vi } from "vitest";
import type { WebSocketClient } from "./client";
import { requestFileContent, requestFileContentAtRef } from "./workspace-files";

function clientReturning(response: Record<string, unknown>) {
  return {
    request: vi.fn().mockResolvedValue(response),
  } as unknown as WebSocketClient;
}

describe("workspace file content", () => {
  function textResponseClient() {
    return clientReturning({
      path: "src/Main.kt",
      content: "fun main() = Unit",
      size: 17,
    });
  }

  it("normalizes omitted text classification for current content", async () => {
    const response = await requestFileContent(textResponseClient(), "session-1", "src/Main.kt");

    expect(response.is_binary).toBe(false);
  });

  it("normalizes omitted text classification for content at a ref", async () => {
    const response = await requestFileContentAtRef(
      textResponseClient(),
      "session-1",
      "src/Main.kt",
      "HEAD~1",
    );

    expect(response.is_binary).toBe(false);
  });

  it("preserves an explicit binary classification", async () => {
    const client = clientReturning({
      path: "image.bin",
      content: "AAE=",
      size: 2,
      is_binary: true,
    });

    await expect(requestFileContent(client, "session-1", "image.bin")).resolves.toMatchObject({
      is_binary: true,
    });
  });
});
