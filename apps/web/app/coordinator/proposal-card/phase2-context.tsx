"use client";

import { createContext, useContext } from "react";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";

export type Phase2CardContextValue = {
  /** The phase-2 flag is on for this page. */
  enabled: boolean;
  /** Standing orders (with retired) once the page's read succeeded; undefined while loading or failed. */
  orders: StandingOrder[] | undefined;
  /** Offers to keep a rejection reason as a standing order. */
  offerReject: ((offer: { proposalId: string; reason: string }) => void) | undefined;
};

const DISABLED: Phase2CardContextValue = {
  enabled: false,
  orders: undefined,
  offerReject: undefined,
};

const Phase2CardContext = createContext<Phase2CardContextValue>(DISABLED);

export const Phase2CardProvider = Phase2CardContext.Provider;

export function usePhase2CardContext(): Phase2CardContextValue {
  return useContext(Phase2CardContext);
}
