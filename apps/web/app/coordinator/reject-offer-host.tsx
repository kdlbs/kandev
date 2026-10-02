"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { AddStandingOrderDialog } from "@/components/coordinators/add-standing-order-dialog";
import { useToast } from "@/components/toast-provider";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";

export const MAX_OFFER_CODE_POINTS = 500;

export type RejectOffer = { proposalId: string; reason: string };

/** The reason, trimmed and cut to the order limit in code points. */
export function offerPrefill(reason: string): string {
  return Array.from(reason.trim()).slice(0, MAX_OFFER_CODE_POINTS).join("");
}

type RejectOfferHostProps = {
  workspaceId: string;
  coordinatorId: string;
  offer: RejectOffer | null;
  onClose: () => void;
  onAdded: (order: StandingOrder) => void;
};

/**
 * Owns the "Make it a standing order" dialog. It lives on the Needs-you page,
 * not in a card, because the card unmounts once its proposal settles.
 */
export function RejectOfferHost({
  workspaceId,
  coordinatorId,
  offer,
  onClose,
  onAdded,
}: RejectOfferHostProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  if (!offer) return null;
  return (
    <AddStandingOrderDialog
      key={offer.proposalId}
      workspaceId={workspaceId}
      coordinatorId={coordinatorId}
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      initialText={offerPrefill(offer.reason)}
      sourceProposalId={offer.proposalId}
      onAdded={(order) => {
        toast({ title: t("coordinator:standingOrderAddedToast"), variant: "success" });
        onAdded(order);
      }}
    />
  );
}

/** The page's single held offer; a later `offer` call replaces an earlier one. */
export function useRejectOffer() {
  const [offer, setOffer] = useState<RejectOffer | null>(null);
  const offerReject = useCallback((next: RejectOffer) => setOffer(next), []);
  const close = useCallback(() => setOffer(null), []);
  return { offer, offerReject, close };
}
