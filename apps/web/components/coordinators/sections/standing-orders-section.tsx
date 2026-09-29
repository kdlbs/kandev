"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { ApiError } from "@/lib/api/client";
import {
  restoreStandingOrder,
  retireStandingOrder,
  type StandingOrder,
} from "@/lib/api/domains/coordinator-api";
import { useStandingOrders } from "@/hooks/domains/coordinator/use-standing-orders";
import { toast } from "@/lib/toast/sonner";
import { AddStandingOrderDialog } from "../add-standing-order-dialog";
import { StandingOrderRow } from "./standing-order-row";

const UNDO_MS = 10_000;

type StandingOrdersSectionProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
};

function useOrderActions(
  workspaceId: string,
  coordinatorId: string,
  orders: StandingOrder[],
  reload: () => void,
) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set());
  const ordersRef = useRef(orders);
  ordersRef.current = orders;

  const setOrderBusy = (id: string, on: boolean) =>
    setBusy((prev) => {
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  const undo = async (id: string) => {
    if (busy.has(id)) return;
    setOrderBusy(id, true);
    try {
      await restoreStandingOrder(workspaceId, coordinatorId, id);
    } catch (error) {
      const isLimit = error instanceof ApiError && error.errorCode === "standing_order_limit";
      const notFound = error instanceof ApiError && error.status === 404;
      if (isLimit) toast.error(t("coordinator:standingOrderLimit"));
      else if (notFound) toast.error(t("coordinator:standingOrderUndoNotFound"));
      else toast.error(t("coordinator:standingOrderUndoFailed"));
    } finally {
      setOrderBusy(id, false);
      reload();
    }
  };

  const retire = async (order: StandingOrder) => {
    if (busy.has(order.id)) return;
    setOrderBusy(order.id, true);
    try {
      await retireStandingOrder(workspaceId, coordinatorId, order.id);
      const stillListed = ordersRef.current.some((item) => item.id === order.id);
      if (stillListed) {
        toast.success(t("coordinator:standingOrderRetired", { number: order.number }), {
          duration: UNDO_MS,
          action: { label: t("coordinator:standingOrderUndo"), onClick: () => void undo(order.id) },
        });
      }
    } catch {
      toast.error(t("coordinator:standingOrderRetireFailed"));
    } finally {
      setOrderBusy(order.id, false);
      reload();
    }
  };

  return { busy, retire };
}

export function StandingOrdersSection({
  workspaceId,
  coordinatorId,
  canManage,
}: StandingOrdersSectionProps) {
  const { t } = useTranslation();
  const { orders, status, reload, retry } = useStandingOrders(workspaceId, coordinatorId);
  const { busy, retire } = useOrderActions(workspaceId, coordinatorId, orders, reload);
  const [dialogOpen, setDialogOpen] = useState(false);

  if (status === "loading") {
    return (
      <div className="animate-pulse space-y-2" data-testid="standing-orders-loading">
        <div className="h-16 rounded-md bg-muted" />
        <div className="h-16 rounded-md bg-muted" />
      </div>
    );
  }
  if (status === "error") {
    return (
      <div className="space-y-3 text-sm text-muted-foreground" data-testid="standing-orders-error">
        <p>{t("coordinator:loadError")}</p>
        <Button type="button" variant="outline" className="cursor-pointer" onClick={retry}>
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-4" data-testid="standing-orders-section">
      {orders.length === 0 ? (
        <p className="text-sm text-muted-foreground" data-testid="standing-orders-empty">
          {t("coordinator:standingOrdersEmpty")}
        </p>
      ) : (
        <ul className="space-y-2">
          {orders.map((order) => (
            <StandingOrderRow
              key={order.id}
              order={order}
              canManage={canManage}
              retiring={busy.has(order.id)}
              onRetire={retire}
            />
          ))}
        </ul>
      )}
      {canManage && (
        <>
          <p className="text-sm text-muted-foreground">
            {t("coordinator:standingOrdersFreshNote")}
          </p>
          <Button
            type="button"
            data-testid="standing-order-add"
            className="min-h-12 cursor-pointer sm:min-h-9"
            onClick={() => setDialogOpen(true)}
          >
            {t("coordinator:standingOrderAdd")}
          </Button>
          <AddStandingOrderDialog
            key={dialogOpen ? "open" : "closed"}
            workspaceId={workspaceId}
            coordinatorId={coordinatorId}
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            onAdded={reload}
          />
        </>
      )}
    </div>
  );
}
