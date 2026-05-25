import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { DatabaseStatsCard } from "@/components/settings/system/database-stats-card";
import { fetchDatabaseStats } from "@/lib/api/domains/system-api";

export default async function SystemDatabasePage() {
  let initialState: Record<string, unknown> = {};
  try {
    const database = await fetchDatabaseStats({ cache: "no-store" }).catch(() => null);
    initialState = { system: database ? { database } : undefined };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title="Database"
        description="SQLite path and size, plus VACUUM, optimize, and factory reset."
      >
        <DatabaseStatsCard />
      </SystemPageShell>
    </StateProvider>
  );
}
