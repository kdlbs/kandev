import { createContext, useContext, useMemo, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useDebounce } from "@/hooks/use-debounce";
import { useControlInboxReads } from "@/hooks/domains/coordinator/use-control-inbox";
import { useIntegratedControl } from "./integrated-control-provider";
import {
  countInbox,
  workspaceInboxFigures,
  type ControlInboxCounts,
  type ControlInboxWorkspaceRead,
  type WorkspaceInboxFigures,
} from "./control-inbox-model";

export type ControlInbox = {
  enabled: boolean;
  loading: boolean;
  refreshing: boolean;
  failed: boolean;
  reads: ControlInboxWorkspaceRead[];
  counts: ControlInboxCounts;
  byWorkspace: Record<string, WorkspaceInboxFigures>;
};

const Context = createContext<ControlInbox | null>(null);
const NO_IDS: string[] = [];
const TICK_SETTLE_MS = 1500;

/**
 * One cross-workspace Needs-you read shared by the control navigation count,
 * the summary attention rows and the inbox view. It follows the control
 * snapshot's revision and the Needs-you refresh tick an answered row bumps.
 */
export function ControlInboxProvider({ children }: { children: ReactNode }) {
  const control = useIntegratedControl();
  const enabled = useFeature("needsYouInbox");
  // Session state events bump this tick in bursts; one settled re-read covers a burst.
  const tick = useDebounce(
    useAppStore((state) => state.needsYouInbox.refreshTick),
    TICK_SETTLE_MS,
  );
  const data = control?.active ? control.data : undefined;
  const ids = useMemo(() => data?.workspaces.map((workspace) => workspace.id) ?? NO_IDS, [data]);
  // A new object per snapshot or tick is the re-read signal useControlResource expects.
  const revision = useMemo(
    () => (enabled && data ? { data, tick } : undefined),
    [enabled, data, tick],
  );
  const result = useControlInboxReads(ids, revision);
  const value = useMemo<ControlInbox>(
    () => ({
      enabled,
      loading: Boolean(revision) && result.status === "loading",
      refreshing: Boolean(result.refreshing),
      failed: result.status !== "ready" && result.status !== "loading",
      reads: result.reads,
      counts: countInbox(result.reads),
      byWorkspace: workspaceInboxFigures(result.reads),
    }),
    [enabled, revision, result],
  );
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useControlInbox() {
  return useContext(Context);
}
