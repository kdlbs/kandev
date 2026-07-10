import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { LogViewer } from "@/components/settings/system/log-viewer";
import { fetchLogFiles } from "@/lib/api/domains/system-api";

export default async function SystemLogsPage() {
  let initialState: Record<string, unknown> = {};
  try {
    const files = await fetchLogFiles({ cache: "no-store" }).catch(() => null);
    if (files) {
      initialState = {
        system: {
          logs: { files, tail: [], tailLoaded: false },
        },
      };
    }
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title="Logs"
        description="Tail of the active backend log file plus rotated downloads."
      >
        <LogViewer />
      </SystemPageShell>
    </StateProvider>
  );
}
