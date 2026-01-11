'use client';

import { useEffect } from 'react';
import type { AppState } from '@/lib/state/store';
import { useAppStoreApi } from '@/components/state-provider';

type StateHydratorProps = {
  initialState: Partial<AppState>;
};

export function StateHydrator({ initialState }: StateHydratorProps) {
  const store = useAppStoreApi();

  useEffect(() => {
    if (Object.keys(initialState).length) {
      store.getState().hydrate(initialState);
    }
  }, [initialState, store]);

  return null;
}
