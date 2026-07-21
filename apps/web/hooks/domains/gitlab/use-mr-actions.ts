"use client";

import { useCallback, useState } from "react";
import { useToast } from "@/components/toast-provider";

export function useMRActions(onRefresh: () => void) {
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const { toast } = useToast();

  const run = useCallback(
    async (label: string, action: () => Promise<unknown>, success: string) => {
      setPendingAction(label);
      try {
        await action();
        toast({ description: success, variant: "success" });
        onRefresh();
        return true;
      } catch (error) {
        toast({
          title: `${label} failed`,
          description: error instanceof Error ? error.message : "GitLab rejected the action.",
          variant: "error",
        });
        return false;
      } finally {
        setPendingAction(null);
      }
    },
    [onRefresh, toast],
  );

  return { pendingAction, run };
}
