'use client';

import {
  createContext,
  useContext,
  useCallback,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react';
import type { CommandItem } from './types';

type CommandRegistryContextValue = {
  register: (sourceId: string, commands: CommandItem[]) => void;
  unregister: (sourceId: string) => void;
  subscribe: (callback: () => void) => () => void;
  getSnapshot: () => CommandItem[];
  open: boolean;
  setOpen: (open: boolean) => void;
};

const CommandRegistryContext = createContext<CommandRegistryContextValue | null>(null);

export function CommandRegistryProvider({ children }: { children: ReactNode }) {
  const commandsRef = useRef(new Map<string, CommandItem[]>());
  const listenersRef = useRef(new Set<() => void>());
  const snapshotRef = useRef<CommandItem[]>([]);
  const [open, setOpen] = useState(false);

  const notify = useCallback(() => {
    // Rebuild snapshot
    const all: CommandItem[] = [];
    for (const items of commandsRef.current.values()) {
      all.push(...items);
    }
    snapshotRef.current = all;
    for (const listener of listenersRef.current) {
      listener();
    }
  }, []);

  const register = useCallback(
    (sourceId: string, commands: CommandItem[]) => {
      commandsRef.current.set(sourceId, commands);
      notify();
    },
    [notify]
  );

  const unregister = useCallback(
    (sourceId: string) => {
      commandsRef.current.delete(sourceId);
      notify();
    },
    [notify]
  );

  const subscribe = useCallback((callback: () => void) => {
    listenersRef.current.add(callback);
    return () => {
      listenersRef.current.delete(callback);
    };
  }, []);

  const getSnapshot = useCallback(() => snapshotRef.current, []);

  return (
    <CommandRegistryContext.Provider
      value={{ register, unregister, subscribe, getSnapshot, open, setOpen }}
    >
      {children}
    </CommandRegistryContext.Provider>
  );
}

export function useCommandRegistry() {
  const ctx = useContext(CommandRegistryContext);
  if (!ctx) throw new Error('useCommandRegistry must be used within CommandRegistryProvider');
  return ctx;
}

export function useCommands(): CommandItem[] {
  const { subscribe, getSnapshot } = useCommandRegistry();
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

export function useCommandPanelOpen() {
  const { open, setOpen } = useCommandRegistry();
  return { open, setOpen };
}
