import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { BackupsTable } from "@/components/settings/system/backups-table";
import { fetchBackups } from "@/lib/api/domains/system-api";

export default async function SystemBackupsPage() {
  let initialState: Record<string, unknown> = {};
  try {
    const backups = await fetchBackups({ cache: "no-store" }).catch(() => null);
    if (backups) {
      initialState = {
        system: {
          backups: { items: backups, loaded: true },
        },
      };
    }
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title="Backups"
        description="VACUUM INTO snapshots stored under <data-dir>/backups/."
      >
        <BackupsTable />
      </SystemPageShell>
    </StateProvider>
  );
}
