import { describe, expect, it, vi } from "vitest";

import {
  createFolderPickerAdapter,
  createNativeFolderPickerAdapter,
  isTauriWebview,
  NativeFolderPickerUnavailableError,
} from "./folder-picker";

function transport(invoke: (command: string) => Promise<unknown>) {
  return { isAvailable: () => true, invoke };
}

describe("native folder picker adapter", () => {
  it("returns the selected directory from the narrow command", async () => {
    const invoke = vi.fn(async () => ({ status: "selected", path: "/Users/example/Code" }));
    const adapter = createNativeFolderPickerAdapter(transport(invoke), () => true);

    await expect(adapter.pickDirectory()).resolves.toEqual({
      status: "selected",
      path: "/Users/example/Code",
    });
    expect(invoke).toHaveBeenCalledWith("pick_directory");
  });

  it("preserves cancellation as an outcome", async () => {
    const adapter = createNativeFolderPickerAdapter(
      transport(async () => ({ status: "cancelled" })),
      () => true,
    );

    await expect(adapter.pickDirectory()).resolves.toEqual({ status: "cancelled" });
  });

  it("does not use the bridge when the boot capability is absent", async () => {
    const invoke = vi.fn(async () => ({ status: "selected", path: "/tmp" }));
    const adapter = createNativeFolderPickerAdapter(transport(invoke), () => false);

    await expect(adapter.pickDirectory()).rejects.toBeInstanceOf(
      NativeFolderPickerUnavailableError,
    );
    expect(invoke).not.toHaveBeenCalled();
  });

  it("distinguishes a browser from a Tauri WebView", () => {
    expect(isTauriWebview({} as Window)).toBe(false);
    expect(isTauriWebview({ __TAURI_INTERNALS__: { invoke: vi.fn() } } as unknown as Window)).toBe(
      true,
    );
  });

  it("does not select the native adapter for an ordinary browser", () => {
    const adapter = createFolderPickerAdapter(
      {} as Window,
      transport(async () => null),
    );

    expect(adapter.isAvailable()).toBe(false);
  });
});
