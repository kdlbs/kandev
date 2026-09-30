import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  SettingsSaveProvider,
  useSettingsSaveContributor,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "./settings-save-provider";

function deferred() {
  let resolve: () => void = () => undefined;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

let coordinator: SettingsSaveCoordinator | null = null;

function Harness({ onSave }: { onSave: () => Promise<void> }) {
  coordinator = useSettingsSaveCoordinator();
  useSettingsSaveContributor({
    id: "draft",
    revision: 1,
    isDirty: true,
    save: onSave,
    discard: () => undefined,
  });
  return null;
}

function mount(onSave: () => Promise<void>) {
  render(
    <SettingsSaveProvider>
      <Harness onSave={onSave} />
    </SettingsSaveProvider>,
  );
}

afterEach(() => {
  cleanup();
  coordinator = null;
});

describe("SettingsSaveProvider exclusive operations", () => {
  it("refuses an exclusive operation while a save is in flight", async () => {
    const gate = deferred();
    mount(() => gate.promise);
    const exclusive = vi.fn(async () => "done");
    let saving: Promise<unknown> = Promise.resolve();
    act(() => {
      saving = coordinator!.saveAll();
    });
    let result: { value: string } | null = { value: "unset" };
    await act(async () => {
      result = await coordinator!.runExclusive(exclusive);
    });
    expect(result).toBeNull();
    expect(exclusive).not.toHaveBeenCalled();
    await act(async () => {
      gate.resolve();
      await saving;
    });
  });

  it("refuses a save while an exclusive operation is in flight", async () => {
    const save = vi.fn(async () => undefined);
    mount(save);
    const gate = deferred();
    let running: Promise<unknown> = Promise.resolve();
    act(() => {
      running = coordinator!.runExclusive(() => gate.promise);
    });
    expect(coordinator!.exclusiveBusy).toBe(true);
    let saved: { canLeave: boolean } = { canLeave: true };
    await act(async () => {
      saved = await coordinator!.saveAll();
    });
    expect(saved.canLeave).toBe(false);
    expect(save).not.toHaveBeenCalled();
    await act(async () => {
      gate.resolve();
      await running;
    });
    expect(coordinator!.exclusiveBusy).toBe(false);
  });

  it("refuses a second exclusive operation and releases after failure", async () => {
    mount(async () => undefined);
    const gate = deferred();
    let first: Promise<unknown> = Promise.resolve();
    act(() => {
      first = coordinator!.runExclusive(() => gate.promise);
    });
    let second: unknown = "unset";
    await act(async () => {
      second = await coordinator!.runExclusive(async () => 1);
    });
    expect(second).toBeNull();
    await act(async () => {
      gate.resolve();
      await first;
    });
    await act(async () => {
      await expect(
        coordinator!.runExclusive(async () => {
          throw new Error("boom");
        }),
      ).rejects.toThrow("boom");
    });
    expect(coordinator!.exclusiveBusy).toBe(false);
  });
});
