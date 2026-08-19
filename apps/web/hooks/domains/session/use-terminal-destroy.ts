"use client";

import { useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";

import { destroyUserShell } from "@/lib/api/domains/user-shell-api";
import { toast } from "@/lib/toast/sonner";

type UseTerminalDestroyOptions = {
  environmentId: string | null | undefined;
  taskId: string | null | undefined;
  onDestroyed: (terminalId: string) => void;
};

export function useTerminalDestroy({
  environmentId,
  taskId,
  onDestroyed,
}: UseTerminalDestroyOptions) {
  const { t } = useTranslation();
  const closingRef = useRef<Set<string>>(new Set());

  const destroyTerminal = useCallback(
    async (terminalId: string): Promise<boolean> => {
      if (!environmentId || closingRef.current.has(terminalId)) return false;

      closingRef.current.add(terminalId);
      onDestroyed(terminalId);

      try {
        await destroyUserShell(environmentId, terminalId, taskId ?? undefined);
        return true;
      } catch {
        toast.error(t("task:failedToCloseTerminal"));
        return false;
      } finally {
        closingRef.current.delete(terminalId);
      }
    },
    [environmentId, taskId, onDestroyed, t],
  );

  return { destroyTerminal };
}
