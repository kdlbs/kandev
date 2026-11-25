"use client";

import { type ReactNode } from "react";
import { WorkerPoolContextProvider } from "@pierre/diffs/react";
import { PIERRE_THEME } from "@/lib/theme/colors";

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
      {children}
    </WorkerPoolContextProvider>
  );
}
