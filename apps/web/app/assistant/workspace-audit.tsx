import { useTranslation } from "react-i18next";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import { useWorkspacePage } from "@/hooks/domains/orchestration/use-assistant-workspaces";
import { AssistantPanel } from "./assistant-panel";
export function WorkspaceAudit({
  binding,
  revision,
}: {
  binding: AssistantBinding;
  revision: number;
}) {
  const { t } = useTranslation();
  const exports = useWorkspacePage("exports", binding, revision);
  return (
    <details className="rounded-md border p-3">
      <summary className="cursor-pointer max-md:min-h-11">
        {t("orchestration:workspaceAudit")}
      </summary>
      <div className="pt-3">
        <AssistantPanel
          title={t("orchestration:workspaceAudit")}
          {...exports}
          count={exports.entries.length}
          onRefresh={() => void exports.refresh()}
          onMore={() => void exports.loadMore()}
        >
          <p className="text-xs text-muted-foreground">{t("orchestration:workspaceAuditHint")}</p>
          <ul className="space-y-2">
            {exports.entries.map((row) => (
              <li key={row.id} className="text-xs rounded-md border p-2 space-y-1 break-all">
                <p>{row.workspace_id}</p>
                <p>{t(`orchestration:workspaceExport_${row.kind}`)}</p>
                <p>
                  {t("orchestration:workspaceProfile", {
                    id: row.receiver_profile_id,
                    revision: row.grant_revision,
                  })}
                </p>
                <time dateTime={row.updated_at}>{new Date(row.updated_at).toLocaleString()}</time>
              </li>
            ))}
          </ul>
        </AssistantPanel>
      </div>
    </details>
  );
}
