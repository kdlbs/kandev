import { createStore, type StoreApi } from "zustand";
import type { AppState } from "./store";

type Commands = Record<string, Promise<void>>;
const commandsByStore = new WeakMap<StoreApi<AppState>, StoreApi<Commands>>();

/** Outstanding dev commands share the app store's lifetime, not a mounted control's. */
export function getDevServerCommands(store: StoreApi<AppState>): StoreApi<Commands> {
  let commands = commandsByStore.get(store);
  if (!commands) {
    commands = createStore<Commands>(() => ({}));
    commandsByStore.set(store, commands);
  }
  return commands;
}

export function runDevServerCommand(
  commands: StoreApi<Commands>,
  sessionId: string,
  run: () => Promise<void>,
): Promise<void> {
  const existing = commands.getState()[sessionId];
  if (existing) return existing;

  let finish!: () => void;
  const promise = new Promise<void>((resolve) => {
    finish = resolve;
  });
  // Admission precedes transport execution and synchronous subscriber notifications.
  commands.setState({ ...commands.getState(), [sessionId]: promise }, true);
  void run()
    .catch(() => {
      // Process status remains authoritative when a command fails.
    })
    .finally(() => {
      const remaining = { ...commands.getState() };
      delete remaining[sessionId];
      commands.setState(remaining, true);
      finish();
    });
  return promise;
}
