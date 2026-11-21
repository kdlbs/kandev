import { createContext, useContext, useState, type ReactNode } from "react";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useIntegratedControlData } from "@/hooks/domains/coordinator/use-integrated-control-data";
import {
  useControlAggregate,
  type ControlAggregateRead,
} from "@/hooks/domains/coordinator/use-control-aggregate";
import { useAppStore } from "@/components/state-provider";
import { isIntegratedControlPath } from "@/lib/navigation/integrated-control";
import { usePathname } from "@/lib/routing/client-router";

type ControlContext = ReturnType<typeof useIntegratedControlData> & {
  active: boolean;
  coordinatorEnabled: boolean;
  /** The Office multi-workspace aggregate; disabled when Office is off. */
  officeEnabled: boolean;
  aggregate: ControlAggregateRead;
  selectedSessions: Record<string, string>;
  selectSession: (taskId: string, sessionId: string) => void;
};
const Context = createContext<ControlContext | null>(null);

export function IntegratedControlProvider({ children }: { children: ReactNode }) {
  const enabled = useFeature("integratedControl");
  const coordinatorEnabled = useFeature("coordinator");
  const officeEnabled = useFeature("office");
  const pathname = usePathname();
  const authenticated = useAppStore(
    (state) => state.auth.mode === "disabled" || state.auth.authenticated,
  );
  const active = enabled && authenticated && isIntegratedControlPath(pathname);
  const reads = useIntegratedControlData(active, coordinatorEnabled);
  const aggregate = useControlAggregate(active && officeEnabled);
  const [selectedSessions, setSelectedSessions] = useState<Record<string, string>>({});
  return (
    <Context.Provider
      value={{
        ...reads,
        refresh: async () => {
          await Promise.all([reads.refresh(), aggregate.refresh()]);
        },
        active,
        coordinatorEnabled,
        officeEnabled,
        aggregate,
        selectedSessions,
        selectSession: (taskId, sessionId) =>
          setSelectedSessions((previous) => ({ ...previous, [taskId]: sessionId })),
      }}
    >
      {children}
    </Context.Provider>
  );
}

export function useIntegratedControl() {
  return useContext(Context);
}
