'use client';

import { useCallback, useMemo } from 'react';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import type { Layout } from 'react-resizable-panels';

const buildDefaultLayout = (ids: string[], base: Record<string, number>): Layout => {
  const raw = ids.map((id) => base[id] ?? 0);
  const total = raw.reduce((sum, value) => sum + value, 0) || 1;
  const normalized = raw.map((value) => (value / total) * 100);

  const result: Layout = {};
  ids.forEach((id, index) => {
    result[id] = normalized[index];
  });
  return result;
};

const cookieKeyForId = (id: string) => `layout:${encodeURIComponent(id)}`;

type UseDefaultLayoutParams = {
  id: string;
  panelIds?: string[];
  baseLayout?: Record<string, number>;
  serverDefaultLayout?: Layout;
};

export function useDefaultLayout({
  id,
  panelIds,
  baseLayout,
  serverDefaultLayout,
}: UseDefaultLayoutParams) {
  const defaultLayout = useMemo(() => {
    const stored = getLocalStorage(id, null as unknown as Layout | null);
    if (stored && typeof stored === 'object' && !Array.isArray(stored)) {
      if (!panelIds || panelIds.every((panelId) => typeof stored[panelId] === 'number')) {
        return stored;
      }
    }
    if (serverDefaultLayout && typeof serverDefaultLayout === 'object') {
      if (!panelIds || panelIds.every((panelId) => typeof serverDefaultLayout[panelId] === 'number')) {
        return serverDefaultLayout;
      }
    }
    if (panelIds && baseLayout) {
      return buildDefaultLayout(panelIds, baseLayout);
    }
    return undefined;
  }, [id, panelIds, baseLayout, serverDefaultLayout]);

  const onLayoutChanged = useCallback(
    (layout: Layout) => {
      if (panelIds && !panelIds.every((panelId) => typeof layout[panelId] === 'number')) return;
      setLocalStorage(id, layout);
      if (typeof document !== 'undefined') {
        document.cookie = `${cookieKeyForId(id)}=${encodeURIComponent(
          JSON.stringify(layout)
        )}; path=/;`;
      }
    },
    [id, panelIds]
  );

  return { defaultLayout, onLayoutChanged };
}
