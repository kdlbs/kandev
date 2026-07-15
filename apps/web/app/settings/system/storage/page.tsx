import { StorageMaintenanceSettings } from "@/components/settings/system/storage/storage-maintenance-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function StoragePage() {
  return (
    <SystemPageShell
      title="Storage"
      description="Analyze disk use and safely maintain Kandev-owned workspaces, caches, and Docker resources."
    >
      <StorageMaintenanceSettings />
    </SystemPageShell>
  );
}
