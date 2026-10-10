import { afterEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { registerWsHandlers } from "@/lib/ws/router";
import type { CustomPrompt } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({ listPrompts: vi.fn() }));
vi.mock("@/lib/api", () => ({ listPrompts: mocks.listPrompts }));
const prompt = (content: string): CustomPrompt => ({
  id: "prompt",
  name: "review",
  content,
  builtin: false,
  created_at: "2026-01-01",
  updated_at: "2026-01-01",
});
function changed(store: ReturnType<typeof createAppStore>) {
  const registration = registerWsHandlers(store);
  const handler = (
    registration.handlers as Record<string, ((message: unknown) => void) | undefined>
  )["prompts.changed"];
  expect(handler).toBeTypeOf("function");
  handler!({ id: "change", type: "notification", action: "prompts.changed", payload: {} });
  registration.dispose();
}

describe("prompt invalidation", () => {
  afterEach(() => {
    mocks.listPrompts.mockReset();
    vi.useRealTimers();
  });
  it("refreshes already-loaded prompts after a remote mutation", async () => {
    const store = createAppStore();
    store.getState().setPrompts([prompt("Old")]);
    mocks.listPrompts.mockResolvedValue({ prompts: [prompt("New")] });
    changed(store);
    await vi.waitFor(() => expect(store.getState().prompts.items[0].content).toBe("New"));
    expect(store.getState().prompts.loading).toBe(false);
  });
  it("discards a stale response and refreshes invalidation received during a request", async () => {
    const store = createAppStore();
    store.getState().setPrompts([prompt("Cached")]);
    let complete!: (value: { prompts: CustomPrompt[] }) => void;
    mocks.listPrompts.mockReturnValueOnce(
      new Promise((resolve) => {
        complete = resolve;
      }),
    );
    mocks.listPrompts.mockResolvedValueOnce({ prompts: [prompt("Latest")] });
    changed(store);
    await vi.waitFor(() => expect(mocks.listPrompts).toHaveBeenCalledTimes(1));
    changed(store);
    expect(mocks.listPrompts).toHaveBeenCalledTimes(1);
    complete({ prompts: [prompt("Stale")] });
    await vi.waitFor(() => expect(store.getState().prompts.items[0].content).toBe("Latest"));
    expect(mocks.listPrompts).toHaveBeenCalledTimes(2);
  });
  it("retries a transient refresh failure without a second change notification", async () => {
    vi.useFakeTimers();
    const store = createAppStore();
    store.getState().setPrompts([prompt("Cached")]);
    mocks.listPrompts.mockRejectedValueOnce(new Error("offline"));
    mocks.listPrompts.mockResolvedValueOnce({ prompts: [prompt("Recovered")] });
    changed(store);
    await vi.advanceTimersByTimeAsync(0);
    expect(store.getState().prompts.items[0].content).toBe("Cached");
    expect(store.getState().prompts.loaded).toBe(false);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(store.getState().prompts.items[0].content).toBe("Recovered");
    expect(store.getState().prompts.loaded).toBe(true);
    expect(mocks.listPrompts).toHaveBeenCalledTimes(2);
  });
  it("bounds failed retries while retaining a retryable cache", async () => {
    vi.useFakeTimers();
    const store = createAppStore();
    store.getState().setPrompts([prompt("Cached")]);
    mocks.listPrompts.mockRejectedValue(new Error("offline"));
    changed(store);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(mocks.listPrompts).toHaveBeenCalledTimes(3);
    expect(store.getState().prompts.items[0].content).toBe("Cached");
    expect(store.getState().prompts.loaded).toBe(false);
    expect(store.getState().prompts.loading).toBe(false);
    mocks.listPrompts.mockResolvedValueOnce({ prompts: [prompt("Recovered")] });
    changed(store);
    await vi.advanceTimersByTimeAsync(0);
    expect(store.getState().prompts.items[0].content).toBe("Recovered");
  });
});
