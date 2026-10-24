"use client";

import { useEffect, useMemo, type ReactNode } from "react";
import { WorkerPoolContextProvider, useWorkerPool } from "@pierre/diffs/react";
import { PIERRE_THEME } from "@/lib/theme/colors";

import { useTheme } from "@/components/theme/app-theme";
import { registerPierreTheme, updatePierreTheme } from "@/lib/theme/pierre-theme";
import { useOptionalAppStoreApi } from "@/components/state-provider";
import { t } from "@/lib/i18n";

function ThemeSync({ children }: { children: ReactNode }) {
  const { effectiveTheme } = useTheme();
  const pool = useWorkerPool();
  const store = useOptionalAppStoreApi();
  useMemo(() => registerPierreTheme(effectiveTheme), [effectiveTheme]);
  useEffect(() => {
    if (!pool) return;
    let current = true;
    void updatePierreTheme(pool, effectiveTheme).catch(() => {
      if (current) store?.getState().setThemeError(t("plugins:failedToLoadPlugins"));
    });
    return () => {
      current = false;
    };
  }, [pool, effectiveTheme, store]);
  return children;
}

const workerFactory = () =>
  new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), { type: "module" });

export function DiffWorkerPoolProvider({ children }: { children: ReactNode }) {
  return (
    <WorkerPoolContextProvider
      poolOptions={{ workerFactory, poolSize: 1 }}
      highlighterOptions={{
        // Don't pre-load language grammars at startup — they resolve on-demand per file type
        // via resolveLanguagesAndExecuteTask. Pre-loading 15 grammars blocks worker pool
        // initialization on cold CI/Docker starts (can take >60s), causing diff views to
        // remain empty until all imports complete.
        langs: [],
        // Each viewer selects its color scheme from the same dual-theme tokens.
        theme: PIERRE_THEME,
        lineDiffType: "word",
      }}
    >
      <ThemeSync>{children}</ThemeSync>
    </WorkerPoolContextProvider>
  );
}
