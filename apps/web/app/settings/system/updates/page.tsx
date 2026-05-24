import { Suspense } from "react";
import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { UpdatesCard } from "@/components/settings/system/updates-card";
import { ChangelogList } from "@/components/settings/changelog-list";
import { fetchUpdates } from "@/lib/api/domains/system-api";

export default async function SystemUpdatesPage() {
  let initialState: Record<string, unknown> = {};
  try {
    const updates = await fetchUpdates({ cache: "no-store" }).catch(() => null);
    initialState = { system: updates ? { updates } : undefined };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title="Updates"
        description="Current vs latest release plus the full kandev changelog."
      >
        <UpdatesCard />
        {/*
         * ChangelogList reads ?page=N via useSearchParams, which forces the
         * client subtree to deopt out of static prerender. The Suspense
         * boundary lets the rest of the page prerender while the list
         * hydrates on the client.
         */}
        <Suspense fallback={null}>
          <ChangelogList />
        </Suspense>
      </SystemPageShell>
    </StateProvider>
  );
}
