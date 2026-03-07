"use client";

import { type ReactNode, useEffect } from "react";
import { useTheme } from "next-themes";
import { WorkerPoolContextProvider, useWorkerPool } from "@pierre/diffs/react";
import { PIERRE_THEME } from "@/lib/theme/colors";

const workerFactory = () =>
  new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), { type: "module" });

function ThemeSync() {
  const { resolvedTheme } = useTheme();
  const pool = useWorkerPool();
  useEffect(() => {
    pool?.setRenderOptions({
      theme: resolvedTheme === "dark" ? PIERRE_THEME.dark : PIERRE_THEME.light,
    });
  }, [pool, resolvedTheme]);
  return null;
}

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
        theme: PIERRE_THEME.dark,
        lineDiffType: "word",
      }}
    >
      <ThemeSync />
      {children}
    </WorkerPoolContextProvider>
  );
}
