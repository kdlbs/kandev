"use client";

import { useRef, useState } from "react";
import type { TFunction } from "i18next";
import {
  addDesktopDiscoveryRootAction,
  confirmHomeDesktopDiscoveryAction,
  reconnectDesktopDiscoveryRootAction,
  removeDesktopDiscoveryRootAction,
} from "@/app/actions/workspaces";
import { useToast } from "@/components/toast-provider";
import type { RepositoryDiscoveryReadKind } from "./use-repository-discovery";

type DiscoveryRefresh = {
  refresh: () => Promise<unknown>;
  synchronizeAfterRootMutation: (kind: RepositoryDiscoveryReadKind) => Promise<unknown>;
};

type Toast = ReturnType<typeof useToast>["toast"];

function reportDiscoveryError(toast: Toast, t: TFunction, error: unknown) {
  toast({
    title: t("workspaces:failedToDiscoverRepositories"),
    description: error instanceof Error ? error.message : t("common:requestFailed"),
    variant: "error",
  });
}

async function runDiscoveryAction(
  action: () => Promise<unknown>,
  synchronize: () => Promise<unknown>,
  toast: Toast,
  t: TFunction,
): Promise<void> {
  try {
    await action();
    await synchronize();
  } catch (error) {
    reportDiscoveryError(toast, t, error);
  }
}

export function useDiscoveryRootActions(discovery: DiscoveryRefresh, toast: Toast, t: TFunction) {
  const confirmingHomeRef = useRef(false);
  const mutationInFlightRef = useRef(false);
  const [isConfirmingHomeDiscovery, setIsConfirmingHomeDiscovery] = useState(false);
  const [isMutating, setIsMutating] = useState(false);

  const startMutation = () => {
    if (mutationInFlightRef.current) return false;
    mutationInFlightRef.current = true;
    setIsMutating(true);
    return true;
  };

  const finishMutation = () => {
    mutationInFlightRef.current = false;
    setIsMutating(false);
  };

  const executeMutatingAction = async (runner: () => Promise<void>) => {
    if (!startMutation()) return;
    try {
      await runner();
    } finally {
      finishMutation();
    }
  };

  const refreshDiscovery = () =>
    executeMutatingAction(() =>
      runDiscoveryAction(() => Promise.resolve(), discovery.refresh, toast, t),
    );
  const handleChooseDiscoveryRoot = (path: string) =>
    executeMutatingAction(() =>
      runDiscoveryAction(
        () => addDesktopDiscoveryRootAction(path),
        () => discovery.synchronizeAfterRootMutation("load"),
        toast,
        t,
      ),
    );
  const handleConfirmHomeDiscovery = async () => {
    if (confirmingHomeRef.current || !startMutation()) return;
    confirmingHomeRef.current = true;
    setIsConfirmingHomeDiscovery(true);
    try {
      await confirmHomeDesktopDiscoveryAction();
      await discovery.synchronizeAfterRootMutation("load");
    } catch (error) {
      reportDiscoveryError(toast, t, error);
    } finally {
      confirmingHomeRef.current = false;
      setIsConfirmingHomeDiscovery(false);
      finishMutation();
    }
  };
  const handleReconnectDiscoveryRoot = (oldPath: string, newPath: string) =>
    executeMutatingAction(() =>
      runDiscoveryAction(
        () => reconnectDesktopDiscoveryRootAction(oldPath, newPath),
        () => discovery.synchronizeAfterRootMutation("load"),
        toast,
        t,
      ),
    );
  const handleRemoveDiscoveryRoot = (path: string) =>
    executeMutatingAction(() =>
      runDiscoveryAction(
        () => removeDesktopDiscoveryRootAction(path),
        () => discovery.synchronizeAfterRootMutation("refresh"),
        toast,
        t,
      ),
    );

  return {
    isMutating,
    refreshDiscovery,
    handleChooseDiscoveryRoot,
    handleConfirmHomeDiscovery,
    isConfirmingHomeDiscovery,
    handleReconnectDiscoveryRoot,
    handleRemoveDiscoveryRoot,
  };
}
