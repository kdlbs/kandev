import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import { useAssistantActions } from "./use-assistant-actions";
const api = vi.hoisted(() => ({ control: vi.fn(), select: vi.fn() }));
vi.mock("@/lib/api/domains/assistant-api", () => ({
  controlAssistant: api.control,
  selectAssistant: api.select,
}));
const binding = {
  id: "b",
  version: 2,
  intent_revision: 4,
  execution_mode: "inspect",
  orchestrator_id: "coordinator",
} as AssistantBinding;
beforeEach(() => {
  api.control.mockReset();
  api.select.mockReset();
});
it("replays an ambiguous stop and uses its receipt revision for the next page", async () => {
  api.control
    .mockRejectedValueOnce(new Error("acknowledgement lost"))
    .mockResolvedValueOnce({
      intent_revision: 5,
      next_cursor: "scoped-next-page",
      partial: true,
      sessions: [{ session_id: "first", status: "unknown" }],
    })
    .mockResolvedValueOnce({ intent_revision: 6, sessions: [] });
  const { result, rerender } = renderHook(
    ({ intent }) => useAssistantActions({ ...binding, intent_revision: intent }, vi.fn()),
    { initialProps: { intent: 4 } },
  );
  await act(async () => {
    await Promise.all([
      result.current.control("stop_managed_work"),
      result.current.control("stop_managed_work"),
    ]);
  });
  expect(api.control).toHaveBeenCalledTimes(1);
  rerender({ intent: 5 });
  await act(async () => {
    await result.current.control("stop_managed_work");
  });
  expect(api.control.mock.calls[1]).toEqual(api.control.mock.calls[0]);
  expect(result.current.receipt?.partial).toBe(true);
  await act(async () => {
    await result.current.control("stop_managed_work", "scoped-next-page");
  });
  expect(api.control.mock.calls[2][0]).toMatchObject({
    expected_intent_revision: 5,
    expected_binding_version: 2,
    after: "scoped-next-page",
  });
  expect(api.control.mock.calls[2][0].operation_id).not.toBe(
    api.control.mock.calls[0][0].operation_id,
  );
});
it("refreshes after a rejected pause and allows a fresh attempt with the new intent", async () => {
  api.control
    .mockRejectedValueOnce(new ApiError("stale_intent", 409, {}))
    .mockResolvedValueOnce({ paused: true });
  const refresh = vi.fn();
  const { result, rerender } = renderHook(
    ({ intent }) => useAssistantActions({ ...binding, intent_revision: intent }, refresh),
    { initialProps: { intent: 4 } },
  );
  await act(async () => {
    await result.current.control("pause");
  });
  expect(refresh).toHaveBeenCalledOnce();
  rerender({ intent: 7 });
  await act(async () => {
    await result.current.control("pause");
  });
  expect(api.control.mock.calls[1][0].expected_intent_revision).toBe(7);
  expect(api.control.mock.calls[1][0].operation_id).not.toBe(
    api.control.mock.calls[0][0].operation_id,
  );
});
