import { useAppStore } from "@/components/state-provider";
import { fileViewerExtension } from "@/lib/plugins/file-viewer-selection";
import { rememberFileViewerReturn } from "@/lib/plugins/file-viewer-return";
import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
export function FindFileViewer({
  path,
  repo,
  sessionId,
}: {
  path: string;
  repo?: string;
  sessionId?: string | null;
}) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const owner = sessionId ?? activeSessionId;
  const extension = fileViewerExtension(path);
  const { t } = useTranslation();
  const query = new URLSearchParams({ tab: "browse" });
  if (extension) query.set("extension", extension);
  return (
    <Button className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11" asChild variant="outline">
      <Link
        href={`/settings/plugins?${query}`}
        onNavigated={() => {
          if (owner) rememberFileViewerReturn(owner, path, repo);
        }}
      >
        {t("plugins:viewerFindPlugin")}
      </Link>
    </Button>
  );
}
