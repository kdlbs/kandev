"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";
import type { AppStatusBarOrderState } from "@/lib/state/slices/settings/types";
import {
  moveAppStatusItem,
  projectActiveStatusItems,
  reconcileAppStatusBarOrder,
  type AppStatusBarSide,
  type AppStatusItemDescriptor,
} from "./app-status-bar-order";

const syncAppStatusBarOrder = createQueuedUserSettingsSync<AppStatusBarOrderState>((order) => ({
  app_status_bar_order: {
    left_item_ids: order.leftItemIds,
    right_item_ids: order.rightItemIds,
  },
}));

export function useAppStatusBarOrder<T extends AppStatusItemDescriptor>(activeItems: T[]) {
  const savedOrder = useAppStore((state) => state.userSettings.appStatusBarOrder);
  const store = useAppStoreApi();
  const [optimisticOrder, setOptimisticOrder] = useState<AppStatusBarOrderState | null>(null);
  const latestRevision = useRef(0);
  const reconciledOrder = useMemo(
    () => reconcileAppStatusBarOrder(savedOrder, activeItems),
    [activeItems, savedOrder],
  );
  const order = optimisticOrder ?? reconciledOrder;
  const projected = useMemo(
    () => projectActiveStatusItems(order, activeItems),
    [activeItems, order],
  );

  const moveItem = useCallback(
    (itemId: string, side: AppStatusBarSide, activeIndex: number) => {
      const next = moveAppStatusItem(order, itemId, side, activeIndex, activeItems);
      const revision = ++latestRevision.current;
      setOptimisticOrder(next);
      void syncAppStatusBarOrder(next)
        .then(() => {
          const state = store.getState();
          state.setUserSettings({ ...state.userSettings, appStatusBarOrder: next });
          if (latestRevision.current === revision) setOptimisticOrder(null);
        })
        .catch(() => {
          if (latestRevision.current !== revision) return;
          setOptimisticOrder(null);
          toast.error("Could not save status bar order");
        });
    },
    [activeItems, order, store],
  );

  return { projected, moveItem };
}
