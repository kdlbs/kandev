import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";

const listTaskSessionMessages = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessionMessages }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

import { useSessionPrompts } from "./use-session-prompts";

function wrapper({ children }: { children: React.ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

describe("useSessionPrompts metadata selectors", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false, cursor: null });
  });

  it("does not produce an uncached snapshot for a session without metadata", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    renderHook(() => useSessionPrompts("session-without-metadata"), { wrapper });

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));

    expect(consoleError).not.toHaveBeenCalledWith(
      expect.stringContaining("The result of getSnapshot should be cached"),
    );
    consoleError.mockRestore();
  });
});
