import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import { listTriggerTypes } from "@/lib/api/domains/automation-api";
import { useTriggerTypeMetadata } from "./use-automation-trigger-types";
import type { TriggerTypeInfo } from "@/lib/types/automation";

let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
}));
vi.mock("@/lib/api/domains/automation-api", () => ({ listTriggerTypes: vi.fn() }));
beforeEach(() => {
  store = createAppStore();
  vi.clearAllMocks();
});

it("keeps out-of-order workspace responses in their own workspace", async () => {
  const pending = new Map<string, (items: TriggerTypeInfo[]) => void>();
  vi.mocked(listTriggerTypes).mockImplementation(
    (id) => new Promise((resolve) => pending.set(id!, resolve)),
  );
  const { result, rerender } = renderHook(({ workspace }) => useTriggerTypeMetadata(workspace), {
    initialProps: { workspace: "a" },
  });
  rerender({ workspace: "b" });
  const b = [{ type: "plugin_event", label: "B" }] as TriggerTypeInfo[];
  await act(async () => pending.get("b")!(b));
  expect(result.current).toEqual(b);
  await act(async () =>
    pending.get("a")!([{ type: "plugin_event", label: "A" }] as TriggerTypeInfo[]),
  );
  expect(result.current).toEqual(b);
});
it("shares a pending enumeration between consumers", async () => {
  let resolve!: (items: TriggerTypeInfo[]) => void;
  vi.mocked(listTriggerTypes).mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const first = renderHook(() => useTriggerTypeMetadata("a"));
  const second = renderHook(() => useTriggerTypeMetadata("a"));
  expect(listTriggerTypes).toHaveBeenCalledTimes(1);
  const types = [{ type: "webhook" }] as TriggerTypeInfo[];
  await act(async () => resolve(types));
  await waitFor(() => expect(first.result.current).toEqual(types));
  expect(second.result.current).toEqual(types);
});
