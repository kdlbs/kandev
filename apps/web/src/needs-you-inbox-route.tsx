import { lazy, Suspense } from "react";
import { AuthRouteRedirect, RouteLoading } from "./spa-route-chrome";

const NeedsYouInboxPageClient = lazy(() =>
  import("@/app/needs-you-inbox/needs-you-inbox-page-client").then((mod) => ({
    default: mod.NeedsYouInboxPageClient,
  })),
);

export function NeedsYouInboxRoute({ enabled }: { enabled: boolean }) {
  if (!enabled) return <AuthRouteRedirect />;
  return (
    <Suspense fallback={<RouteLoading routeNameKey="sidebar:needsYouInbox" />}>
      <NeedsYouInboxPageClient />
    </Suspense>
  );
}
