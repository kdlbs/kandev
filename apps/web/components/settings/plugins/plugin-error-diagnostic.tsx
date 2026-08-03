import type { PluginRecord } from "@/lib/types/plugins";

/**
 * Renders the host's persisted lifecycle diagnostic without inventing a
 * second error state in the UI. The message is already normalized by the
 * backend; the timestamp remains machine-readable for assistive tools and
 * browser inspection.
 */
export function PluginErrorDiagnostic({ plugin }: { plugin: PluginRecord }) {
  if (!plugin.last_error) return null;

  return (
    <div
      role="alert"
      data-testid={`plugin-error-${plugin.id}`}
      className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive [overflow-wrap:anywhere]"
    >
      <span>{plugin.last_error}</span>
      {plugin.last_error_at && (
        <time
          dateTime={plugin.last_error_at}
          className="mt-1 block text-[11px] text-destructive/70"
        >
          {new Date(plugin.last_error_at).toLocaleString()}
        </time>
      )}
    </div>
  );
}
