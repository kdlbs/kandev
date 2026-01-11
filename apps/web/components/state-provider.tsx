'use client';

import { createContext, useContext, useRef } from 'react';
import type { StoreApi } from 'zustand';
import { useStore } from 'zustand';
import type { AppState, StoreProviderProps } from '@/lib/state/store';
import { createAppStore } from '@/lib/state/store';

const StoreContext = createContext<StoreApi<AppState> | null>(null);

export function StateProvider({ children, initialState }: StoreProviderProps) {
  const storeRef = useRef<StoreApi<AppState> | null>(null);

  if (!storeRef.current) {
    storeRef.current = createAppStore(initialState);
  }

  return <StoreContext.Provider value={storeRef.current}>{children}</StoreContext.Provider>;
}

export function useAppStore<T>(selector: (state: AppState) => T) {
  const store = useContext(StoreContext);
  if (!store) {
    throw new Error('useAppStore must be used within StateProvider');
  }
  return useStore(store, selector);
}

export function useAppStoreApi() {
  const store = useContext(StoreContext);
  if (!store) {
    throw new Error('useAppStoreApi must be used within StateProvider');
  }
  return store;
}
