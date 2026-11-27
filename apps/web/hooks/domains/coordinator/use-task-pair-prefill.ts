"use client";

import { useEffect } from "react";
import {
  prefillTaskPair,
  type TaskPair,
  type TaskPairTouched,
} from "@/lib/coordinator/prefill-task-pair";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

type Args = {
  workspaceDefaultAgentProfileId: string | null | undefined;
  /** `undefined` until the agent profiles have loaded. */
  agentProfiles: readonly AgentProfileOption[] | undefined;
  ownAgent: string;
  ownExecutor: string;
  touched: TaskPairTouched;
  current: TaskPair;
  onChange: (pair: TaskPair) => void;
};

/**
 * Keeps the untouched fields of Agent for created tasks in step with the
 * workspace default and the coordinator's own pair while one is being added.
 */
export function useTaskPairPrefill({ onChange, ...input }: Args) {
  const { current } = input;
  const next = prefillTaskPair(input);
  useEffect(() => {
    if (next.agent !== current.agent || next.executor !== current.executor) onChange(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed on the computed values
  }, [next.agent, next.executor, current.agent, current.executor]);
}
