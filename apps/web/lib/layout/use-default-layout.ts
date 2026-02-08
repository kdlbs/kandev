'use client';

import { useCallback, useMemo } from 'react';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import type { Layout } from 'react-resizable-panels';

const MIN_PANEL_PERCENT = 5;

const buildDefaultLayout = (ids: string[], base: Record<string, number>): Layout => {
  // For unknown panel IDs, assign the average of known values so they get reasonable space
  const knownValues = ids.map((id) => base[id]).filter((v): v is number => v != null && v > 0);
  const fallback = knownValues.length > 0
    ? knownValues.reduce((a, b) => a + b, 0) / knownValues.length
    : 50;

  const raw = ids.map((id) => base[id] ?? fallback);
  const total = raw.reduce((sum, value) => sum + value, 0) || 1;
  const normalized = raw.map((value) => (value / total) * 100);

  const result: Layout = {};
  ids.forEach((id, index) => {
    result[id] = normalized[index];
  });
  return result;
};

const isLayoutValid = (layout: Layout, panelIds: string[]): boolean => {
  return panelIds.every(
    (id) => typeof layout[id] === 'number' && layout[id] >= MIN_PANEL_PERCENT
  );
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
      if (!panelIds || isLayoutValid(stored, panelIds)) {
        return stored;
      }
    }
    if (serverDefaultLayout && typeof serverDefaultLayout === 'object') {
      if (!panelIds || isLayoutValid(serverDefaultLayout, panelIds)) {
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
