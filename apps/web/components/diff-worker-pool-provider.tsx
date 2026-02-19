"use client";

import { type ReactNode, useEffect } from "react";
import { useTheme } from "next-themes";
import { WorkerPoolContextProvider, useWorkerPool } from "@pierre/diffs/react";
import { PIERRE_THEME } from "@/lib/theme/colors";

const LANGS = [
  "javascript",
  "typescript",
  "jsx",
  "tsx",
  "json",
  "css",
  "html",
  "markdown",
  "python",
  "go",
  "rust",
  "java",
  "yaml",
  "toml",
  "bash",
  "sql",
] as const;

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
      poolOptions={{ workerFactory, poolSize: 4 }}
      highlighterOptions={{
        langs: [...LANGS],
        theme: PIERRE_THEME.dark,
        lineDiffType: "word",
      }}
    >
      <ThemeSync />
      {children}
    </WorkerPoolContextProvider>
  );
}
