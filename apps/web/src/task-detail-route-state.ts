import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import type {
  ActiveTurnWindowObservation,
  TaskSessionHydrationEpoch,
} from "@/lib/state/slices/session/types";
import type {
  TaskNavigationContext,
  TaskNavigationIdentity,
} from "@/lib/state/task-navigation-reads";

export type TaskDetailRouteState =
  | { routeKey: string; status: "loading"; data: null }
  | {
      routeKey: string;
      status: "loaded";
      data: FetchedSessionData;
      forceMergeSession: boolean;
      navigationContext?: TaskNavigationContext;
      turnWindowObservationsAtRequestStart?: Readonly<Record<string, ActiveTurnWindowObservation>>;
      hydrationEpochsAtRequestStart?: Readonly<Record<string, TaskSessionHydrationEpoch>>;
      enrichmentIdentity?: TaskNavigationIdentity;
    }
  | { routeKey: string; status: "error"; data: null };
