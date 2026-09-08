"use client";

import type { TFunction } from "i18next";
import {
  addDesktopDiscoveryRootAction,
  reconnectDesktopDiscoveryRootAction,
  removeDesktopDiscoveryRootAction,
} from "@/app/actions/workspaces";
import { useToast } from "@/components/toast-provider";

type DiscoveryRefresh = {
  refresh: () => Promise<unknown>;
  load: () => Promise<unknown>;
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
  const refreshDiscovery = () =>
    runDiscoveryAction(() => Promise.resolve(), discovery.refresh, toast, t);
  const handleChooseDiscoveryRoot = (path: string) =>
    runDiscoveryAction(() => addDesktopDiscoveryRootAction(path), discovery.load, toast, t);
  const handleReconnectDiscoveryRoot = (oldPath: string, newPath: string) =>
    runDiscoveryAction(
      () => reconnectDesktopDiscoveryRootAction(oldPath, newPath),
      discovery.load,
      toast,
      t,
    );
  const handleRemoveDiscoveryRoot = (path: string) =>
    runDiscoveryAction(() => removeDesktopDiscoveryRootAction(path), discovery.refresh, toast, t);

  return {
    refreshDiscovery,
    handleChooseDiscoveryRoot,
    handleReconnectDiscoveryRoot,
    handleRemoveDiscoveryRoot,
  };
}
