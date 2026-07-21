"use client";

import { useCallback, useState } from "react";

import { useToast } from "@/components/toast-provider";

import type { UtilityGenerationResult } from "./use-utility-agent-generator";

type UsePromptResultDeliveryOptions = {
  getCurrent: () => string | null;
  apply: (value: string) => boolean;
};

export type PromptResultDelivery = {
  deliver: (source: string, result: UtilityGenerationResult) => boolean;
  pendingResult: UtilityGenerationResult | null;
  applyPending: () => void;
  copyPending: () => Promise<void>;
  dismissPending: () => void;
};

const INSERT_FAILURE_MESSAGE = "Enhanced prompt was generated but could not be inserted.";
const COPY_SUCCESS_MESSAGE = "Enhanced prompt copied to clipboard.";
const COPY_FAILURE_MESSAGE = "Enhanced prompt could not be copied.";

function fallbackCopy(text: string): boolean {
  if (typeof document === "undefined") {
    return false;
  }

  const textArea = document.createElement("textarea");
  textArea.value = text;
  textArea.style.position = "fixed";
  textArea.style.opacity = "0";
  document.body.appendChild(textArea);
  textArea.focus();
  textArea.select();

  try {
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    document.body.removeChild(textArea);
  }
}

async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      return fallbackCopy(text);
    }
  }

  return fallbackCopy(text);
}

export function usePromptResultDelivery({
  getCurrent,
  apply,
}: UsePromptResultDeliveryOptions): PromptResultDelivery {
  const [pendingResult, setPendingResult] = useState<UtilityGenerationResult | null>(null);
  const { toast } = useToast();

  const retainPendingResult = useCallback(
    (result: UtilityGenerationResult) => {
      setPendingResult(result);
      toast({ description: INSERT_FAILURE_MESSAGE, variant: "error" });
    },
    [toast],
  );

  const deliver = useCallback(
    (source: string, result: UtilityGenerationResult) => {
      if (getCurrent() !== source) {
        retainPendingResult(result);
        return false;
      }

      if (apply(result.content)) {
        setPendingResult(null);
        return true;
      }

      retainPendingResult(result);
      return false;
    },
    [apply, getCurrent, retainPendingResult],
  );

  const applyPending = useCallback(() => {
    setPendingResult((current) => {
      if (!current) {
        return null;
      }

      return apply(current.content) ? null : current;
    });
  }, [apply]);

  const copyPending = useCallback(async () => {
    if (!pendingResult) {
      return;
    }

    const copied = await copyText(pendingResult.content);
    toast({
      description: copied ? COPY_SUCCESS_MESSAGE : COPY_FAILURE_MESSAGE,
      variant: copied ? "success" : "error",
    });
  }, [pendingResult, toast]);

  const dismissPending = useCallback(() => {
    setPendingResult(null);
  }, []);

  return { deliver, pendingResult, applyPending, copyPending, dismissPending };
}
