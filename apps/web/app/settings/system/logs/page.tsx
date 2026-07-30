import { LogViewer } from "@/components/settings/system/log-viewer";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function SystemLogsPage() {
  return (
    <SystemPageShell
      title="Logs"
      description="Download a bounded diagnostic ZIP containing frontend and backend logs."
    >
      <LogViewer />
    </SystemPageShell>
  );
}
