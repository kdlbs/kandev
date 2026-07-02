"use client";

import { useEffect } from "react";

import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";

export default function AutomationsTopLevelPage() {
  const router = useRouter();
  const workspaces = useAppStore((s) => s.workspaces.items);

  const soleWorkspaceId = workspaces.length === 1 ? workspaces[0].id : null;

  useEffect(() => {
    if (soleWorkspaceId) {
      router.replace(`/settings/workspace/${soleWorkspaceId}/automations`);
    }
  }, [soleWorkspaceId, router]);

  if (soleWorkspaceId) {
    return null;
  }

  if (workspaces.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-8 text-center">
        <p className="text-sm font-medium">No workspaces yet</p>
        <p className="mt-1 text-xs text-muted-foreground">
          Create a workspace first, then schedule automations inside it.
        </p>
        <Link
          href="/settings/workspace"
          className="mt-4 inline-block rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer"
        >
          Create workspace
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Automations</h2>
        <p className="text-sm text-muted-foreground">Pick a workspace to manage its automations.</p>
      </div>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {workspaces.map((ws) => (
          <Link
            key={ws.id}
            href={`/settings/workspace/${ws.id}/automations`}
            className="rounded-md border p-3 text-sm hover:bg-muted/50 cursor-pointer"
          >
            <div className="font-medium">{ws.name}</div>
            {ws.description && (
              <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                {ws.description}
              </div>
            )}
          </Link>
        ))}
      </div>
    </div>
  );
}
