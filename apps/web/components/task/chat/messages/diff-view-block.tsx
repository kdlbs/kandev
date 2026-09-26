"use client";

import { lazy, Suspense } from "react";
import { cn } from "@/lib/utils";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { DiffViewInline } from "@/components/diff";
import type { FileDiffData } from "@/lib/diff/types";

const LazyMonacoInlineDiff = lazy(async () => {
  const module = await import("@/components/editors/monaco/monaco-inline-diff");
  return { default: module.MonacoInlineDiff };
});

type DiffViewBlockProps = {
  data: FileDiffData;
  className?: string;
};

export function DiffViewBlock({ data, className }: DiffViewBlockProps) {
  const provider = useEditorProvider("chat-diff");

  if (provider === "monaco") {
    return (
      <Suspense fallback={null}>
        <LazyMonacoInlineDiff data={data} className={className} />
      </Suspense>
    );
  }

  return (
    <div
      className={cn(
        "mt-3 rounded-md border border-border/50 bg-background/60 px-3 text-xs",
        className,
      )}
    >
      <DiffViewInline data={data} />
    </div>
  );
}
