"use client";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { MERMAID_ERROR_EVENT } from "./mermaid-utils";

type ToastFn = ReturnType<typeof useToast>["toast"];
type MermaidErrorDetail = { message: string; taskId?: string | null };

const notifiedTaskIds = new Set<string>();

export function showMermaidErrorToast(
  toast: ToastFn,
  taskId: string | null | undefined,
  message: string,
  title: string,
): void {
  if (taskId) {
    if (notifiedTaskIds.has(taskId)) return;
    notifiedTaskIds.add(taskId);
  }

  toast({ title, description: message, variant: "error" });
}

export function resetMermaidErrorToastHistoryForTest(): void {
  notifiedTaskIds.clear();
}

export function useMermaidErrorToast(): void {
  const { toast } = useToast();
  const { t } = useTranslation("common");

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<MermaidErrorDetail>).detail;
      showMermaidErrorToast(toast, detail?.taskId, detail?.message, t("failedToRenderDiagram"));
    };
    document.addEventListener(MERMAID_ERROR_EVENT, handler);
    return () => document.removeEventListener(MERMAID_ERROR_EVENT, handler);
  }, [t, toast]);
}
