import { StorageMaintenanceSettings } from "@/components/settings/system/storage/storage-maintenance-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function StoragePage() {
  return (
    <SystemPageShell
      title="Storage"
      description="Review disk use and reclaim space from Kandev-owned workspaces, caches, and Docker resources whenever your installation needs it."
    >
      <StorageMaintenanceSettings />
    </SystemPageShell>
  );
}
