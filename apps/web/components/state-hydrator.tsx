'use client';

import { useLayoutEffect } from 'react';
import type { AppState } from '@/lib/state/store';
import { useAppStoreApi } from '@/components/state-provider';

type StateHydratorProps = {
  initialState: Partial<AppState>;
};

export function StateHydrator({ initialState }: StateHydratorProps) {
  const store = useAppStoreApi();

  // Use useLayoutEffect to hydrate state synchronously before child effects run.
  // This ensures SSR-hydrated data is available before hooks like useSettingsData
  // decide whether to fetch data.
  useLayoutEffect(() => {
    if (Object.keys(initialState).length) {
      store.getState().hydrate(initialState);
    }
  }, [initialState, store]);

  return null;
}
