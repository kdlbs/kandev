import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import type {
  ActiveTurnWindowObservation,
  TaskSessionHydrationEpoch,
} from "@/lib/state/slices/session/types";
import type {
  TaskNavigationContext,
  TaskNavigationIdentity,
} from "@/lib/state/task-navigation-reads";
import type { TaskNavigationSelectionOwner } from "@/lib/state/task-navigation-selection";

export type TaskDetailRouteState =
  | { routeKey: string; status: "loading"; data: null }
  | {
      routeKey: string;
      status: "loaded";
      data: FetchedSessionData;
      forceMergeSession: boolean;
      navigationContext?: TaskNavigationContext;
      // Boot selection admission does not grant client read or enrichment ownership.
      bootSelectionOwner?: TaskNavigationSelectionOwner;
      turnWindowObservationsAtRequestStart?: Readonly<Record<string, ActiveTurnWindowObservation>>;
      hydrationEpochsAtRequestStart?: Readonly<Record<string, TaskSessionHydrationEpoch>>;
      enrichmentIdentity?: TaskNavigationIdentity;
    }
  | {
      routeKey: string;
      status: "error";
      data: null;
      temporary?: boolean;
      sessionAdmission?: "unavailable" | "read_failed";
    };
