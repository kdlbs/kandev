"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useToast } from "@/components/toast-provider";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

import type { UtilityGenerationResult } from "./use-utility-agent-generator";
import { t } from "@/lib/i18n";

type UsePromptResultDeliveryOptions = {
  scopeKey: string;
  getCurrent: () => string | null;
  apply: (value: string) => boolean;
};

export type PromptResultDelivery = {
  captureScope: () => number;
  deliver: (source: string, result: UtilityGenerationResult, generation: number) => boolean;
  pendingResult: UtilityGenerationResult | null;
  applyPending: () => void;
  copyPending: () => Promise<void>;
  dismissPending: () => void;
};

// Catalog keys: module scope, so callers resolve them at use.
const INSERT_FAILURE_MESSAGE_KEY = "task:enhancedPromptCouldNotBeInserted";
const COPY_SUCCESS_MESSAGE_KEY = "task:enhancedPromptCopiedToClipboard";
const COPY_FAILURE_MESSAGE_KEY = "task:enhancedPromptCouldNotBeCopied";

type PendingResult = {
  result: UtilityGenerationResult;
  generation: number;
};

export function usePromptResultDelivery({
  scopeKey,
  getCurrent,
  apply,
}: UsePromptResultDeliveryOptions): PromptResultDelivery {
  const [pendingResult, setPendingResult] = useState<PendingResult | null>(null);
  const scopeKeyRef = useRef(scopeKey);
  const generationRef = useRef(0);
  if (scopeKeyRef.current !== scopeKey) {
    scopeKeyRef.current = scopeKey;
    generationRef.current += 1;
  }
  const { toast } = useToast();

  useEffect(
    () => () => {
      generationRef.current += 1;
    },
    [],
  );

  const retainPendingResult = useCallback(
    (result: UtilityGenerationResult, generation: number) => {
      setPendingResult({ result, generation });
      toast({ description: t(INSERT_FAILURE_MESSAGE_KEY), variant: "error" });
    },
    [toast],
  );

  const captureScope = useCallback(() => generationRef.current, []);

  const deliver = useCallback(
    (source: string, result: UtilityGenerationResult, generation: number) => {
      if (generation !== generationRef.current) {
        return false;
      }

      if (getCurrent() !== source) {
        retainPendingResult(result, generation);
        return false;
      }

      if (apply(result.content)) {
        setPendingResult(null);
        return true;
      }

      retainPendingResult(result, generation);
      return false;
    },
    [apply, getCurrent, retainPendingResult],
  );

  const applyPending = useCallback(() => {
    if (
      pendingResult &&
      pendingResult.generation === generationRef.current &&
      apply(pendingResult.result.content)
    ) {
      setPendingResult(null);
    }
  }, [apply, pendingResult]);

  const copyPending = useCallback(async () => {
    if (!pendingResult || pendingResult.generation !== generationRef.current) {
      return;
    }

    const copied = await copyToClipboard(pendingResult.result.content);
    toast({
      description: t(copied ? COPY_SUCCESS_MESSAGE_KEY : COPY_FAILURE_MESSAGE_KEY),
      variant: copied ? "success" : "error",
    });
  }, [pendingResult, toast]);

  const dismissPending = useCallback(() => {
    setPendingResult(null);
  }, []);

  const visiblePendingResult =
    pendingResult?.generation === generationRef.current ? pendingResult.result : null;

  return {
    captureScope,
    deliver,
    pendingResult: visiblePendingResult,
    applyPending,
    copyPending,
    dismissPending,
  };
}
