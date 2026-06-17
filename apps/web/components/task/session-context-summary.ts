import type { RefObject } from "react";
import type { SummarizeSessionResult } from "@/hooks/use-summarize-session";

export type SummaryToastFn = (opts: {
  title: string;
  description?: string;
  variant?: "error" | "default";
}) => void;

export function sanitizePromptText(value: string): string {
  return value.replace(/\r/g, "").replace(/[<>]/g, " ");
}

export function applySummarizeSessionResult({
  result,
  promptRef,
  setContextValue,
  setHasPrompt,
  toast,
}: {
  result: SummarizeSessionResult;
  promptRef: RefObject<HTMLTextAreaElement | null>;
  setContextValue: (v: string) => void;
  setHasPrompt: (v: boolean) => void;
  toast: SummaryToastFn;
}) {
  if (result.summary === null) {
    setContextValue("blank");
    toast({
      title: "Summarize failed",
      description:
        result.error ??
        "Could not generate a summary. Check that the summarize utility agent is configured and enabled in settings.",
      variant: "error",
    });
    return;
  }

  if (promptRef.current) {
    promptRef.current.value = sanitizePromptText(result.summary);
    setHasPrompt(true);
  }
}
