import { redirect } from "@/lib/routing/server-navigation";
import Link from "@/components/routing/app-link";
import { listWorkspaces } from "@/lib/api";

export default async function AutomationsTopLevelPage() {
  const res = await listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] }));
  const workspaces = res.workspaces ?? [];

  if (workspaces.length === 1) {
    redirect(`/settings/workspace/${workspaces[0].id}/automations`);
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
