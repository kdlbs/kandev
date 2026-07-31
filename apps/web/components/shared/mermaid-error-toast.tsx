"use client";

import { useEffect } from "react";
import { useToast } from "@/components/toast-provider";
import { MERMAID_ERROR_EVENT } from "./mermaid-utils";

type ToastFn = ReturnType<typeof useToast>["toast"];
type MermaidErrorDetail = { message: string; taskId?: string | null };

const notifiedTaskIds = new Set<string>();

export function showMermaidErrorToast(
  toast: ToastFn,
  taskId: string | null | undefined,
  message: string,
): void {
  if (taskId) {
    if (notifiedTaskIds.has(taskId)) return;
    notifiedTaskIds.add(taskId);
  }

  toast({ title: "Failed to render diagram", description: message, variant: "error" });
}

export function resetMermaidErrorToastHistoryForTest(): void {
  notifiedTaskIds.clear();
}

export function useMermaidErrorToast(): void {
  const { toast } = useToast();

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<MermaidErrorDetail>).detail;
      showMermaidErrorToast(toast, detail?.taskId, detail?.message);
    };
    document.addEventListener(MERMAID_ERROR_EVENT, handler);
    return () => document.removeEventListener(MERMAID_ERROR_EVENT, handler);
  }, [toast]);
}
