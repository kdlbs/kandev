import { lazy, Suspense } from "react";
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

export function CoordinatorRoute({
  enabled,
  view,
  workspaceId,
  coordinatorId,
}: CoordinatorRouteProps) {
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
