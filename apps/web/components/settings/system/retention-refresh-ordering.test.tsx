import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type { RetentionSettings, RetentionStatus } from "@/lib/types/system";
import { RetentionSettingsCard } from "./retention-settings-card";

let store!: ReturnType<typeof useAppStoreApi>;
let save!: ReturnType<typeof useSettingsSaveCoordinator>;
let reads: Array<(response: Response) => void> = [];
const settings = (batch_limit: number): RetentionSettings => ({
  enabled: true,
  sweep_interval_hours: 6,
  batch_limit,
  routine_runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
  runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
  run_events: { warn_rows: 250000 },
});
const status = (batch: number, skip_count = 0): RetentionStatus => ({
  settings: settings(batch),
  last_sweep: null,
  skip_count,
  retained_counts: {
    office_routine_runs: { state: "not_computed", retained_count: 0, as_of: "" },
    runs: { state: "not_computed", retained_count: 0, as_of: "" },
    run_events: { state: "not_computed", retained_count: 0, as_of: "" },
  },
});
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}
function CaptureSave() {
  save = useSettingsSaveCoordinator();
  return null;
}
function View({ visible = true }: { visible?: boolean }) {
  return (
    <StateProvider>
      <CaptureStore />
      {visible && (
        <SettingsSaveProvider>
          <CaptureSave />
          <RetentionSettingsCard />
        </SettingsSaveProvider>
      )}
    </StateProvider>
  );
}
const field = () => screen.getByTestId("retention-batch-limit") as HTMLInputElement;
const expand = () =>
  fireEvent.click(screen.getByTestId("retention-advanced-settings").querySelector("summary")!);
async function saveBatch(batch: number) {
  fireEvent.change(field(), { target: { value: String(batch) } });
  await act(async () => expect((await save.saveAll()).canLeave).toBe(true));
}

afterEach(async () => {
  cleanup();
  await act(async () => reads.forEach((done) => done(response(status(7000)))));
  reads = [];
  vi.unstubAllGlobals();
  localStorage.clear();
  clearNavigationBlockerForTests();
});

// @covers AC-OFFICE-RUN-HISTORY-RETENTION-004.10
it.each(["old first", "new first", "old failure"])(
  "preserves the acknowledged baseline across route reentry: %s",
  async (order) => {
    let persisted = settings(5000);
    const puts: RetentionSettings[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init: RequestInit) => {
        if (init.method === "PUT") {
          persisted = JSON.parse(String(init.body));
          puts.push(persisted);
          return Promise.resolve(response(persisted));
        }
        return new Promise<Response>((resolve) => reads.push(resolve));
      }),
    );
    const view = render(<View visible={false} />);
    act(() => store.getState().setSystemRetention(status(5000)));
    view.rerender(<View />);
    expand();
    await saveBatch(6000);
    expect(reads).toHaveLength(1);
    view.rerender(<View visible={false} />);
    view.rerender(<View />);
    expand();
    expect(field().disabled).toBe(false);
    expect(field().value).toBe("6000");
    await saveBatch(7000);
    expect(reads).toHaveLength(2);
    expect(puts.map((s) => s.batch_limit)).toEqual([6000, 7000]);
    const old = () =>
      reads[0](
        order === "old failure"
          ? new Response("offline", { status: 500 })
          : response(status(6000, 1)),
      );
    if (order === "old first") {
      await act(async () => old());
      expect(store.getState().system.retention?.settings.batch_limit).toBe(7000);
      expect(field().disabled).toBe(true);
    }
    const latest = status(7000, 2);
    await act(async () => reads[1](response(latest)));
    if (order !== "old first") await act(async () => old());
    expect(persisted.batch_limit).toBe(7000);
    expect(store.getState().system.retention).toEqual(latest);
    expect(field().value).toBe("7000");
    expect(save.hasDirty).toBe(false);
    expect(screen.queryByTestId("retention-save-error")).toBeNull();
    fireEvent.change(field(), { target: { value: "8000" } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Reset" })));
    expect(field().value).toBe("7000");
    expect(save.hasDirty).toBe(false);
    expect(puts).toHaveLength(2);
  },
);

it.each([false, true])(
  "uses the normalized acknowledgement, preserving later edits: %s",
  async (editAgain) => {
    let acknowledge!: (value: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn((_url, init) => {
        if (init.method === "PUT")
          return new Promise<Response>((resolve) => {
            acknowledge = resolve;
          });
        return new Promise<Response>((resolve) => reads.push(resolve));
      }),
    );
    const view = render(<View visible={false} />);
    act(() => store.getState().setSystemRetention(status(5000)));
    view.rerender(<View />);
    expand();
    fireEvent.change(field(), { target: { value: "6001" } });
    let saving!: ReturnType<typeof save.saveAll>;
    act(() => {
      saving = save.saveAll();
    });
    if (editAgain) fireEvent.change(field(), { target: { value: "8000" } });
    await act(async () => {
      acknowledge(response(settings(6000)));
      await saving;
    });
    await act(async () => reads[0](new Response("offline", { status: 500 })));
    expect(store.getState().system.retention?.settings.batch_limit).toBe(6000);
    expect(field().value).toBe(editAgain ? "8000" : "6000");
    expect(save.hasDirty).toBe(editAgain);
    if (editAgain) {
      await act(async () => fireEvent.click(screen.getByRole("button", { name: "Reset" })));
      expect(field().value).toBe("6000");
      expect(save.hasDirty).toBe(false);
    }
  },
);
