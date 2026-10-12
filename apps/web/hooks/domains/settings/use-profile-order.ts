"use client";

import { useCallback, useMemo } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { t as translate } from "@/lib/i18n";
import { getProfileOrderQueue } from "@/lib/settings/profile-order-queue";

export function useProfileOrder() {
  const store = useAppStoreApi();
  const { toast } = useToast();
  const queue = useMemo(
    () =>
      getProfileOrderQueue(store, {
        onError: (error) =>
          toast({
            title: translate("agents:profileOrderSaveFailed"),
            description: error instanceof Error ? error.message : translate("agents:requestFailed"),
            variant: "error",
          }),
      }),
    [store, toast],
  );
  return useCallback(
    (agentId: string, profileIds: string[]) => {
      queue.requestProfileOrder(agentId, profileIds);
    },
    [queue],
  );
}
