import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { AssistantBinding, AssistantInputSnapshot } from "@/lib/api/domains/assistant-api";
import { useAssistantInput } from "./use-assistant-input";
const api = vi.hoisted(() => ({ get: vi.fn(), resolve: vi.fn() }));
vi.mock("@/lib/api/domains/assistant-api", () => ({
  getAssistantInput: api.get,
  resolveAssistantInput: api.resolve,
}));
const binding = { id: "b", version: 2, intent_revision: 4 } as AssistantBinding;
const snapshot = {
  attention: { id: "a", revision: 5, source_revision: "native-source-v2" },
  input: { session_id: "session", pending_id: "pending" },
} as AssistantInputSnapshot;
beforeEach(() => {
  api.get.mockReset().mockResolvedValue(snapshot);
  api.resolve.mockReset().mockResolvedValue({});
});
it("retries an ambiguous answer with the original operation and intent, suppressing double click", async () => {
  api.resolve.mockRejectedValueOnce(new Error("lost acknowledgement"));
  const onResolved = vi.fn();
  const { result, rerender } = renderHook(
    ({ intent }) =>
      useAssistantInput({ ...binding, intent_revision: intent }, snapshot.attention, onResolved),
    { initialProps: { intent: 4 } },
  );
  await waitFor(() => expect(result.current.snapshot).toBeDefined());
  await act(async () => {
    await Promise.all([
      result.current.resolve({ option_id: "allow-once" }),
      result.current.resolve({ option_id: "allow-once" }),
    ]);
  });
  expect(api.resolve).toHaveBeenCalledTimes(1);
  expect(onResolved).not.toHaveBeenCalled();
  rerender({ intent: 5 });
  await act(async () => {
    await result.current.resolve({ option_id: "allow-once" });
  });
  expect(api.resolve.mock.calls[1]).toEqual(api.resolve.mock.calls[0]);
  expect(api.resolve.mock.calls[0][1]).toMatchObject({
    expected_intent_revision: 4,
    expected_binding_version: 2,
    expected_revision: 5,
    source_revision: "native-source-v2",
    session_id: "session",
  });
});
it("preserves native skip reasons and refuses a mismatched native pending handle", async () => {
  const { result } = renderHook(() => useAssistantInput(binding, snapshot.attention, vi.fn()));
  await waitFor(() => expect(result.current.snapshot).toBeDefined());
  await expect(result.current.transport.respond("foreign", { rejected: true })).resolves.toEqual({
    state: "expired",
  });
  expect(api.resolve).not.toHaveBeenCalled();
  await act(async () => {
    await result.current.transport.respond("pending", {
      rejected: true,
      reject_reason: "Use the original task",
    });
  });
  expect(api.resolve.mock.calls[0][1]).toMatchObject({
    rejected: true,
    reject_reason: "Use the original task",
  });
  expect(api.resolve.mock.calls[0][1]).not.toHaveProperty("reason");
});
it("refetches after a definite conflict and captures current intent for the next attempt", async () => {
  api.resolve.mockRejectedValueOnce(new ApiError("stale_intent", 409, {}));
  const onResolved = vi.fn();
  const { result, rerender } = renderHook(
    ({ intent }) =>
      useAssistantInput({ ...binding, intent_revision: intent }, snapshot.attention, onResolved),
    { initialProps: { intent: 4 } },
  );
  await waitFor(() => expect(result.current.snapshot).toBeDefined());
  await act(async () => {
    await result.current.resolve({ option_id: "deny" });
  });
  rerender({ intent: 6 });
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2));
  await act(async () => {
    await result.current.resolve({ option_id: "deny" });
  });
  expect(api.resolve.mock.calls[1][1].expected_intent_revision).toBe(6);
  expect(api.resolve.mock.calls[1][1].operation_id).not.toBe(
    api.resolve.mock.calls[0][1].operation_id,
  );
  expect(onResolved).toHaveBeenCalled();
});
it("does not expose input delivered after its owner view was disposed", async () => {
  let deliver!: (value: AssistantInputSnapshot) => void;
  api.get.mockImplementationOnce(
    () =>
      new Promise<AssistantInputSnapshot>((resolve) => {
        deliver = resolve;
      }),
  );
  const { result, unmount } = renderHook(() =>
    useAssistantInput(binding, snapshot.attention, vi.fn()),
  );
  const signal = api.get.mock.calls[0][1] as AbortSignal;
  unmount();
  await act(async () => {
    deliver(snapshot);
  });
  expect(signal.aborted).toBe(true);
  expect(result.current.snapshot).toBeUndefined();
});
