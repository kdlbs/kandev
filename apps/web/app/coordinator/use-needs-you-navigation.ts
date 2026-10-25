"use client";

import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import type { NeedsYouItem } from "@/lib/coordinator/attention";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { useToast } from "@/components/toast-provider";
import type { ProposalCardForm } from "./proposal-card/proposal-card";

export type NeedsYouFormNavigation = {
  /** The single item id whose form should auto-open this mount, or null. */
  autoOpenProposalId: string | null;
  autoOpenForm: ProposalCardForm | null;
  /** The matching item's `ProposalCard` calls this once it actually opens `autoOpenForm`. */
  onAutoFormOpened: () => void;
};

function parseForm(value: string | null): ProposalCardForm | null {
  return value === "edit" || value === "reject" ? value : null;
}

function clearedSearch(searchParams: URLSearchParams): string {
  const remaining = new URLSearchParams(searchParams);
  remaining.delete("proposal");
  remaining.delete("form");
  return remaining.toString();
}

/**
 * Reads `?proposal=<id>&form=edit|reject` (proposal-cards.md#cards "Forms
 * and navigation"), the chat card's deep link into the Needs-you screen.
 * Once `inputsLoaded`, scrolls the matching item into view and hands it back
 * as the one item to auto-open. An id that is not a current item shows a
 * toast and clears the query params immediately; a matching item instead
 * clears them once its own card reports `onAutoFormOpened`, since the item
 * can reach `items` (and this hook's own `inputsLoaded` gate can pass)
 * before that specific card has mounted with a resolved row — clearing
 * eagerly would hand it a params-already-cleared `autoOpenForm: null` right
 * as it mounts. Either way the query params are cleared with a history
 * replace, once per distinct `proposal` value.
 */
export function useNeedsYouFormNavigation(
  items: NeedsYouItem[],
  inputsLoaded: boolean,
): NeedsYouFormNavigation {
  const { t } = useTranslation();
  const { toast } = useToast();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const proposalParam = searchParams.get("proposal");
  const formParam = parseForm(searchParams.get("form"));
  const handledRef = useRef<string | null>(null);

  const clearParams = useCallback(() => {
    const query = clearedSearch(searchParams);
    router.replace(query ? `${pathname}?${query}` : pathname);
  }, [pathname, router, searchParams]);

  useEffect(() => {
    if (!inputsLoaded || !proposalParam || !formParam) return;
    if (handledRef.current === proposalParam) return;

    const match = items.find((item) => item.id === proposalParam && item.kind === "proposal");
    if (!match) {
      handledRef.current = proposalParam;
      clearParams();
      toast({ title: t("coordinator:toastProposalNoLongerWaiting"), variant: "error" });
      return;
    }
    document.querySelector(`[data-testid="needs-you-item-${match.id}"]`)?.scrollIntoView({
      block: "center",
    });
  }, [inputsLoaded, proposalParam, formParam, items, clearParams, t, toast]);

  const onAutoFormOpened = useCallback(() => {
    if (!proposalParam || handledRef.current === proposalParam) return;
    handledRef.current = proposalParam;
    clearParams();
  }, [proposalParam, clearParams]);

  if (!inputsLoaded || !proposalParam || !formParam) {
    return { autoOpenProposalId: null, autoOpenForm: null, onAutoFormOpened };
  }
  return { autoOpenProposalId: proposalParam, autoOpenForm: formParam, onAutoFormOpened };
}
