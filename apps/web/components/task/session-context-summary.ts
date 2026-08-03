import type { RefObject } from "react";
import type { SummarizeSessionResult } from "@/hooks/use-summarize-session";

type PromptValueTarget = { value: string } | { setValue: (value: string) => void };
type PromptValueRef = RefObject<PromptValueTarget | null>;

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
  promptRef: PromptValueRef;
  setContextValue: (v: string) => void;
  setHasPrompt: (v: boolean) => void;
  toast: SummaryToastFn;
}) {
  const setPromptValue = (value: string): boolean => {
    const target = promptRef.current;
    if (!target) return false;
    if ("setValue" in target) {
      target.setValue(value);
    } else {
      target.value = value;
    }
    return true;
  };

  if (result.summary === null) {
    setContextValue("blank");
    setPromptValue("");
    setHasPrompt(false);
    toast({
      title: "Summarize failed",
      description:
        result.error ??
        "Could not generate a summary. Check that the summarize utility agent is configured and enabled in settings.",
      variant: "error",
    });
    return;
  }

  if (setPromptValue(sanitizePromptText(result.summary))) {
    setHasPrompt(true);
  }
}
