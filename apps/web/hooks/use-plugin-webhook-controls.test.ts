import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { managePluginWebhook } from "@/lib/api/domains/automation-api";
import { useWebhookControls } from "./use-plugin-webhook-controls";
import type { AutomationTrigger } from "@/lib/types/automation";
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:38429" }),
}));
vi.mock("@/lib/api/domains/automation-api", () => ({
  managePluginWebhook: vi.fn(),
  listPluginWebhookReceipts: vi.fn().mockResolvedValue([]),
}));
const trigger = { id: "t", automation_id: "a", updated_at: "1" } as AutomationTrigger;
const binding = { id: "b", trigger_id: "t", path: "/api/v1/automations/webhook-bindings/b" };
beforeEach(() => vi.clearAllMocks());
it("blocks configuration during initial lookup and builds the backend URL", async () => {
  let resolve!: (value: null) => void;
  vi.mocked(managePluginWebhook)
    .mockReturnValueOnce(
      new Promise((done) => {
        resolve = done;
      }),
    )
    .mockResolvedValue(binding);
  const { result } = renderHook(() => useWebhookControls(trigger, false));
  expect(result.current.busy).toBe(true);
  await act(async () => result.current.act("configure"));
  expect(managePluginWebhook).toHaveBeenCalledTimes(1);
  await act(async () => resolve(null));
  await act(async () => result.current.act("configure"));
  expect(result.current.url).toBe("http://localhost:38429" + binding.path);
});
it("ignores an operation response after the trigger changes", async () => {
  let resolve!: (value: typeof binding & { secret: string }) => void;
  vi.mocked(managePluginWebhook).mockResolvedValue(null);
  const { result, rerender } = renderHook(({ value }) => useWebhookControls(value, false), {
    initialProps: { value: trigger },
  });
  await waitFor(() => expect(result.current.busy).toBe(false));
  vi.mocked(managePluginWebhook).mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  let operation!: Promise<void>;
  act(() => {
    operation = result.current.act("reveal");
  });
  rerender({ value: { ...trigger, id: "other" } });
  await act(async () => {
    resolve({ ...binding, secret: "obsolete" });
    await operation;
  });
  expect(result.current.secret).toBeNull();
  expect(result.current.binding).toBeNull();
});
