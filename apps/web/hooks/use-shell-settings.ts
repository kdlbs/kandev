'use client';

import { useAppStore } from '@/components/state-provider';

export function useShellSettings() {
  const preferredShell = useAppStore((state) => state.userSettings.preferredShell);
  const shellOptions = useAppStore((state) => state.userSettings.shellOptions);
  const loaded = useAppStore((state) => state.userSettings.loaded);

  return {
    preferredShell,
    shellOptions,
    loaded,
  };
}
