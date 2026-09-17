import { readBootPayload } from "@/src/boot-payload";

import { createTauriInvokeTransport } from "./tauri-event-transport";
import type { DesktopInvokeTransport } from "./updater-adapter";

export type FolderPickerOutcome =
  | { status: "selected"; path: string }
  | { status: "cancelled" }
  | { status: "failed"; message?: string };

export class NativeFolderPickerUnavailableError extends Error {
  readonly code = "native-folder-picker-unavailable";

  constructor() {
    super();
    this.name = "NativeFolderPickerUnavailableError";
  }
}

export type NativeFolderPickerAdapter = {
  isAvailable: () => boolean;
  pickDirectory: () => Promise<FolderPickerOutcome>;
};

type WindowWithTauri = Window & {
  __TAURI_INTERNALS__?: unknown;
};

function browserWindow(): Window | undefined {
  return typeof window === "undefined" ? undefined : window;
}

export function isTauriWebview(win: Window | undefined = browserWindow()): boolean {
  return Boolean((win as WindowWithTauri | undefined)?.__TAURI_INTERNALS__);
}

function nativeCapability(win: Window | undefined = browserWindow()): boolean {
  if (!win) return false;
  return readBootPayload(win).runtime?.nativeFolderPickerAvailable === true;
}

function parseOutcome(value: unknown): FolderPickerOutcome {
  if (!value || typeof value !== "object") return { status: "failed" };
  const record = value as Record<string, unknown>;
  if (record.status === "cancelled") return { status: "cancelled" };
  if (record.status === "selected" && typeof record.path === "string" && record.path !== "") {
    return { status: "selected", path: record.path };
  }
  if (record.status === "failed") {
    return {
      status: "failed",
      ...(typeof record.message === "string" ? { message: record.message } : {}),
    };
  }
  return { status: "failed" };
}

export function createNativeFolderPickerAdapter(
  transport: DesktopInvokeTransport,
  capability: () => boolean,
): NativeFolderPickerAdapter {
  const isAvailable = () => capability() && transport.isAvailable();
  return {
    isAvailable,
    async pickDirectory() {
      if (!isAvailable()) throw new NativeFolderPickerUnavailableError();
      return parseOutcome(await transport.invoke("pick_directory"));
    },
  };
}

export function createFolderPickerAdapter(
  win: Window | undefined = browserWindow(),
  transport: DesktopInvokeTransport = createTauriInvokeTransport(),
): NativeFolderPickerAdapter {
  return createNativeFolderPickerAdapter(transport, () => nativeCapability(win ?? browserWindow()));
}

export const nativeFolderPicker = createFolderPickerAdapter();
