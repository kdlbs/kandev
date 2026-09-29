"use client";

import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { useStandingOrders } from "@/hooks/domains/coordinator/use-standing-orders";
import { Phase2CardProvider } from "./proposal-card/phase2-context";
import { RejectOfferHost, useRejectOffer } from "./reject-offer-host";
import { pruneStallResumes } from "./use-stall-resume";

type Phase2PageProviderProps = {
  workspaceId: string;
  coordinatorId: string;
  /** Changes whenever the page's proposal list refreshes; reloads the orders read. */
  proposalsKey: string;
  /** Task ids of the stalls now on the page; a held Resume of any other task is dropped. */
  liveStallTaskIds: string[];
  children: ReactNode;
};

/**
 * The phase-2 data every Needs-you card shares: one standing-orders read (with
 * retired orders) for the Shaped by labels, the reject offer host and the
 * held-Resume pruning. Mounted only while the phase-2 flag is on.
 */
export function Phase2PageProvider({
  workspaceId,
  coordinatorId,
  proposalsKey,
  liveStallTaskIds,
  children,
}: Phase2PageProviderProps) {
  const { orders, status, reload } = useStandingOrders(workspaceId, coordinatorId, {
    includeRetired: true,
  });
  const { offer, offerReject, close } = useRejectOffer();

  const firstKey = useRef(proposalsKey);
  useEffect(() => {
    if (firstKey.current === proposalsKey) return;
    firstKey.current = proposalsKey;
    void reload();
  }, [proposalsKey, reload]);

  const stallKey = liveStallTaskIds.join("\n");
  useEffect(() => {
    pruneStallResumes(new Set(stallKey ? stallKey.split("\n") : []));
  }, [stallKey]);

  const value = useMemo(
    () => ({
      enabled: true,
      orders: status === "ready" ? orders : undefined,
      offerReject,
    }),
    [status, orders, offerReject],
  );

  return (
    <Phase2CardProvider value={value}>
      {children}
      <RejectOfferHost
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        offer={offer}
        onClose={close}
        onAdded={() => void reload()}
      />
    </Phase2CardProvider>
  );
}
