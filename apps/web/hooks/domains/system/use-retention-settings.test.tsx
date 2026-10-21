import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { RetentionSettings, RetentionStatus } from "@/lib/types/system";
import { useRetentionSettings } from "./use-retention-settings";

function status(batch = 5000, skip_count = 0): RetentionStatus {
  return {
    settings: {
      enabled: true,
      sweep_interval_hours: 6,
      batch_limit: batch,
      routine_runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
      runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
      run_events: { warn_rows: 250000 },
    },
    last_sweep: null,
    skip_count,
    retained_counts: {
      office_routine_runs: { state: "not_computed", retained_count: 0, as_of: "" },
      runs: { state: "not_computed", retained_count: 0, as_of: "" },
      run_events: { state: "not_computed", retained_count: 0, as_of: "" },
    },
  };
}
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });
const wrapper = ({ children }: { children: ReactNode }) => (
  <StateProvider>{children}</StateProvider>
);
function consumer() {
  return { remote: useRetentionSettings(), store: useAppStoreApi() };
}
function transport() {
  const reads: Array<(value: Response) => void> = [];
  const writes: Array<(value: Response) => void> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(
      (_url, init) =>
        new Promise<Response>((resolve) => {
          (init.method === "PUT" ? writes : reads).push(resolve);
        }),
    ),
  );
  return { reads, writes };
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// @covers AC-OFFICE-RUN-HISTORY-RETENTION-004.10
it.each(["success", "failure"])(
  "ignores old %s and finalization during a newer reload",
  async (outcome) => {
    const { reads } = transport();
    const { result } = renderHook(consumer, { wrapper });
    expect(reads).toHaveLength(1);
    act(() => {
      void result.current.remote.reload();
    });
    expect(reads).toHaveLength(2);
    await act(async () =>
      reads[0](
        outcome === "success" ? response(status(4000)) : new Response("old", { status: 500 }),
      ),
    );
    expect(result.current.remote.status).toBeNull();
    expect(result.current.remote.isLoading).toBe(true);
    expect(result.current.remote.error).toBeNull();
    await act(async () => reads[1](response(status(7000, 3))));
    expect(result.current.remote.status).toEqual(status(7000, 3));
    expect(result.current.remote.isLoading).toBe(false);
  },
);

it("keeps a newer read after an old success, reports a current error and allows retry", async () => {
  const { reads } = transport();
  const { result } = renderHook(consumer, { wrapper });
  act(() => {
    void result.current.remote.reload();
  });
  await act(async () => reads[1](response(status(7000, 4))));
  await act(async () => reads[0](response(status(5000, 1))));
  expect(result.current.remote.status).toEqual(status(7000, 4));
  act(() => {
    void result.current.remote.reload();
  });
  await act(async () => reads[2](new Response("current failure", { status: 500 })));
  expect(result.current.remote.error).toContain("500");
  expect(result.current.remote.status).toEqual(status(7000, 4));
  act(() => {
    void result.current.remote.reload();
  });
  await act(async () => reads[3](response(status(8000, 5))));
  expect(result.current.remote.error).toBeNull();
  expect(result.current.remote.status).toEqual(status(8000, 5));
});

it("shares read ownership between consumers but isolates independent stores", async () => {
  const { reads } = transport();
  const same = renderHook(() => [useRetentionSettings(), useRetentionSettings()], { wrapper });
  const independent = renderHook(consumer, { wrapper });
  expect(reads).toHaveLength(3);
  await act(async () => reads[1](response(status(7000))));
  await act(async () => reads[2](response(status(9000))));
  await act(async () => reads[0](response(status(5000))));
  expect(same.result.current.map((r) => r.status?.settings.batch_limit)).toEqual([7000, 7000]);
  expect(independent.result.current.remote.status?.settings.batch_limit).toBe(9000);
  expect(same.result.current.every((r) => !r.isLoading)).toBe(true);
});

it("publishes an admitted normalized write after unmount without reviving a retired read", async () => {
  const { reads, writes } = transport();
  const { result, unmount } = renderHook(consumer, { wrapper });
  await act(async () => reads[0](response(status(5000, 9))));
  const store = result.current.store;
  const latestStatus = { ...status(5000, 9), last_skip_at: "2026-10-21T00:00:00Z" };
  act(() => store.getState().setSystemRetention(latestStatus));
  let save!: Promise<RetentionSettings>;
  act(() => {
    void result.current.remote.reload();
    save = result.current.remote.save(status(6001).settings);
  });
  unmount();
  await act(async () => {
    writes[0](response(status(6000).settings));
    await save;
  });
  expect(store.getState().system.retention).toEqual({
    ...latestStatus,
    settings: status(6000).settings,
  });
  expect(reads).toHaveLength(2);
  await act(async () => reads[1](response(status(5000))));
  expect(store.getState().system.retention?.settings.batch_limit).toBe(6000);
});

it("does not advance the baseline on failed PUT and accepts a valid pending read", async () => {
  const { reads, writes } = transport();
  const { result } = renderHook(consumer, { wrapper });
  await act(async () => reads[0](response(status())));
  let saving!: Promise<RetentionSettings>;
  act(() => {
    void result.current.remote.reload();
    saving = result.current.remote.save(status(7000).settings);
  });
  await act(async () => {
    writes[0](new Response("write failed", { status: 500 }));
    await expect(saving).rejects.toThrow();
  });
  expect(result.current.remote.saveError).toContain("500");
  expect(result.current.remote.status?.settings.batch_limit).toBe(5000);
  await act(async () => reads[1](response(status(5500, 2))));
  expect(result.current.remote.status).toEqual(status(5500, 2));
  act(() => {
    saving = result.current.remote.save(status(7000).settings);
  });
  await act(async () => {
    writes[1](response(status(7000).settings));
    await saving;
  });
  await act(async () => reads[2](response(status(7000, 3))));
  expect(result.current.remote.saveError).toBeNull();
  expect(result.current.remote.status).toEqual(status(7000, 3));
});
