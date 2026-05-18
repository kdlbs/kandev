import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { HealthIssuesCard } from "@/components/settings/system/health-issues-card";
import { DiskUsageCard } from "@/components/settings/system/disk-usage-card";
import { VersionSummaryCard } from "@/components/settings/system/version-summary-card";
import { fetchSystemHealth } from "@/lib/api/domains/health-api";
import { fetchUpdates } from "@/lib/api/domains/system-api";

export default async function SystemStatusPage() {
  let initialState: Record<string, unknown> = {};
  try {
    const [health, updates] = await Promise.all([
      fetchSystemHealth({ cache: "no-store" }).catch(() => null),
      fetchUpdates({ cache: "no-store" }).catch(() => null),
    ]);
    initialState = {
      systemHealth: health
        ? { healthy: health.healthy, issues: health.issues, loaded: true, loading: false }
        : undefined,
      system: updates ? { updates } : undefined,
    };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell title="Status" description="Health checks, disk usage, and version summary.">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <HealthIssuesCard />
          <VersionSummaryCard />
        </div>
        <DiskUsageCard />
      </SystemPageShell>
    </StateProvider>
  );
}
