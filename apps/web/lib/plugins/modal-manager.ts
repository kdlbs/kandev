/**
 * Reactive singleton backing `host.openModal(...)` (docs/plans/plugins/PLUGIN-API.md).
 *
 * Tracks every open plugin modal as a list of `{ instanceId, pluginId, options }`
 * and exposes a tiny external-store subscription so `<PluginModalHost/>`
 * re-renders whenever a modal opens or closes — same pattern as
 * `lib/plugins/registry.ts`'s `pluginRegistry` singleton.
 */
import { useSyncExternalStore } from "react";
import type { PluginModalHandle, PluginModalOptions } from "./types";

export interface OpenPluginModal {
  instanceId: string;
  pluginId: string;
  options: PluginModalOptions;
}

let nextInstanceId = 0;

class PluginModalManager {
  private modals: OpenPluginModal[] = [];
  private listeners = new Set<() => void>();
  private snapshot: OpenPluginModal[] = [];

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getSnapshot = (): OpenPluginModal[] => this.snapshot;

  /** Opens a modal owned by `pluginId`; returns a handle to close it. */
  openModal(pluginId: string, options: PluginModalOptions): PluginModalHandle {
    const instanceId = `plugin-modal-${(nextInstanceId += 1)}`;
    this.modals = [...this.modals, { instanceId, pluginId, options }];
    this.notify();
    return { close: () => this.close(instanceId) };
  }

  /** Removes every modal owned by `pluginId` (called on plugin disable/unload). */
  closeAllForPlugin(pluginId: string): void {
    const before = this.modals.length;
    this.modals = this.modals.filter((modal) => modal.pluginId !== pluginId);
    if (this.modals.length !== before) this.notify();
  }

  /** Removes a single modal by `instanceId`, e.g. from the host's Dialog `onOpenChange`. */
  close(instanceId: string): void {
    const before = this.modals.length;
    this.modals = this.modals.filter((modal) => modal.instanceId !== instanceId);
    if (this.modals.length !== before) this.notify();
  }

  private notify(): void {
    this.snapshot = this.modals;
    this.listeners.forEach((listener) => listener());
  }
}

export const pluginModalManager = new PluginModalManager();

/** Snapshot hook: re-renders the caller whenever any plugin modal opens/closes. */
export function usePluginModals(): OpenPluginModal[] {
  return useSyncExternalStore(
    pluginModalManager.subscribe,
    pluginModalManager.getSnapshot,
    pluginModalManager.getSnapshot,
  );
}
