"use client";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";

// Takes the route name as a catalog KEY, not resolved copy: a `t()` at the call
// site would sit in a plain route-dispatch function with no hook of its own.
export function RouteLoading({ routeNameKey }: { routeNameKey: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex h-full min-h-0 w-full items-center justify-center bg-background">
      <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
        {t("common:loadingRoute", { routeName: t(routeNameKey) })}
      </p>
    </div>
  );
}

export function AuthRouteRedirect() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/");
  }, [router]);
  return null;
}
