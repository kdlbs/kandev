import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { PluginStatusBadge } from "@/components/settings/plugins/plugin-status-badge";
import { fileViewerExtension } from "@/lib/plugins/file-viewer-selection";

export function UnavailableFileViewers({ path }: { path: string }) {
  const plugins = useAppStore((state) => state.plugins.items);
  const extension = fileViewerExtension(path);
  const matching = plugins.filter(
    (plugin) =>
      plugin.status !== "uninstalled" &&
      plugin.ui?.file_viewers?.some((viewer) => viewer.extensions.includes(extension)),
  );
  return matching.map((plugin) => (
    <div key={plugin.id} className="flex flex-wrap items-center justify-center gap-2">
      <Link
        className="underline break-all"
        href={`/settings/plugins/${encodeURIComponent(plugin.id)}`}
      >
        {plugin.display_name}
      </Link>
      <PluginStatusBadge status={plugin.status} />
    </div>
  ));
}
