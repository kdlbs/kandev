import { lazy, Suspense, useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { AuthRouteRedirect, RouteLoading } from "./spa-route-chrome";

const NeedsYouPageClient = lazy(() =>
  import("@/app/coordinator/needs-you-page-client").then((mod) => ({
    default: mod.NeedsYouPageClient,
  })),
);
const QueuePageClient = lazy(() =>
  import("@/app/coordinator/queue-page-client").then((mod) => ({
    default: mod.QueuePageClient,
  })),
);

export type CoordinatorRouteProps = {
  enabled: boolean;
  view: "needs-you" | "queue";
  workspaceId: string;
  coordinatorId: string | null;
};

function useActivateRouteWorkspace(workspaceId: string, enabled: boolean) {
  const known = useAppStore((state) => state.workspaces.items.some((w) => w.id === workspaceId));
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  useEffect(() => {
    if (enabled && known) setActiveWorkspace(workspaceId);
  }, [enabled, known, workspaceId, setActiveWorkspace]);
}

export function CoordinatorRoute({
  enabled,
  view,
  workspaceId,
  coordinatorId,
}: CoordinatorRouteProps) {
  useActivateRouteWorkspace(workspaceId, enabled);
  if (!enabled) return <AuthRouteRedirect />;
  const PageClient = view === "queue" ? QueuePageClient : NeedsYouPageClient;
  return (
    <Suspense
      fallback={
        <RouteLoading
          routeNameKey={`coordinator:${view === "queue" ? "queueTitle" : "needsYouTitle"}`}
        />
      }
    >
      <PageClient workspaceId={workspaceId} coordinatorId={coordinatorId} />
    </Suspense>
  );
}
