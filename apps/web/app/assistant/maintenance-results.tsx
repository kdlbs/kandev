import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type {
  Improvement,
  MaintenanceGrant,
  MaintenanceSuccess,
} from "@/lib/api/domains/assistant-maintenance-types";
import {
  useMaintenanceArtifact,
  useMaintenanceFile,
  useMaintenancePage,
} from "@/hooks/domains/orchestration/use-assistant-maintenance";
import { AssistantPanel } from "./assistant-panel";
import { MaintenanceSelect } from "./maintenance-select";
type ViewProps = { binding: AssistantBinding; row: Improvement };
export function MaintenanceEvidence({ binding, row }: ViewProps) {
  const { t } = useTranslation();
  const page = useMaintenancePage("evidence", binding, row.id, row.revision);
  return (
    <details className="rounded-md border p-2">
      <summary className="cursor-pointer text-sm max-md:min-h-11">
        {t("orchestration:maintenanceEvidence")}
      </summary>
      <AssistantPanel
        title={t("orchestration:maintenanceEvidence")}
        {...page}
        count={page.entries.length}
        onRefresh={() => void page.refresh()}
        onMore={() => void page.loadMore()}
      >
        <p className="text-xs text-muted-foreground">
          {t("orchestration:maintenanceEvidenceHint")}
        </p>
        {row.policy_version === "unknown" && (
          <p className="text-xs">{t("orchestration:maintenancePolicyUnknown")}</p>
        )}
        <ul className="space-y-2">
          {page.entries.map((item) => (
            <li key={item.id} className="text-xs space-y-1">
              <p>
                {t(`orchestration:maintenanceOrigin_${item.origin}`)} ·{" "}
                {t(`orchestration:maintenanceReason_${item.reason}`)}
              </p>
              <time dateTime={item.observed_at}>{new Date(item.observed_at).toLocaleString()}</time>{" "}
              <Link
                href={`/t/${encodeURIComponent(item.task_id)}?workspaceId=${encodeURIComponent(row.workspace_id)}`}
                className="inline-flex underline cursor-pointer max-md:min-h-11 items-center"
              >
                {t("orchestration:maintenanceAffectedTask")}
              </Link>
            </li>
          ))}
        </ul>
      </AssistantPanel>
    </details>
  );
}
export function MaintenanceArtifactView({ binding, row }: ViewProps) {
  const { t } = useTranslation();
  const state = useMaintenanceArtifact(binding, row.id, row.revision);
  if (!state.data || state.error)
    return (
      <div role={state.error ? "alert" : "status"}>
        <p>{t(state.error ? "orchestration:maintenanceArtifactUnavailable" : "common:loading")}</p>
        <Button
          variant="outline"
          className="cursor-pointer max-md:min-h-11"
          onClick={state.refresh}
        >
          {t("task:retry")}
        </Button>
      </div>
    );
  const data = state.data;
  return (
    <section className="space-y-2 min-w-0" aria-label={t("orchestration:maintenanceArtifact")}>
      <p className="text-xs">{t("orchestration:maintenanceArtifactHint")}</p>
      {data.commit_oid !== data.base_oid && (
        <p className="text-xs break-all">
          <span>{t("orchestration:maintenanceCommitId")}</span> <code>{data.commit_oid}</code>
        </p>
      )}
      {data.patch ? (
        <>
          <pre
            className="rounded-md bg-muted p-2 text-xs overflow-auto max-h-80 whitespace-pre-wrap break-all"
            data-testid="maintenance-patch"
          >
            {data.patch}
          </pre>
          <Button
            variant="outline"
            className="cursor-pointer max-md:min-h-11"
            onClick={() => downloadPatch(data.patch)}
          >
            {t("orchestration:maintenanceDownload")}
          </Button>
        </>
      ) : (
        <p className="text-sm">{t("orchestration:maintenanceNoCommit")}</p>
      )}
    </section>
  );
}
function downloadPatch(patch: string) {
  const url = URL.createObjectURL(new Blob([patch], { type: "text/x-diff;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "kandev-repair.patch";
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function MaintenanceFileView({
  binding,
  row,
  grant,
}: ViewProps & { grant: MaintenanceGrant }) {
  const { t } = useTranslation();
  const [path, setPath] = useState("");
  const state = useMaintenanceFile(binding, row.id, grant.revision, path);
  return (
    <details className="rounded-md border p-2 min-w-0">
      <summary className="cursor-pointer text-sm max-md:min-h-11">
        {t("orchestration:maintenanceFilePreview")}
      </summary>
      <div className="space-y-2">
        <MaintenanceSelect
          label={t("orchestration:maintenanceFiles")}
          value={path}
          options={grant.scope.files.map((file) => ({ id: file, name: file }))}
          onChange={setPath}
        />
        {Boolean(state.error) && (
          <p role="alert" className="text-sm">
            {t("orchestration:assistantUnavailable")}
          </p>
        )}
        {state.data && (
          <pre className="rounded-md bg-muted p-2 text-xs overflow-auto max-h-80 whitespace-pre-wrap break-all">
            {state.data.content}
          </pre>
        )}
      </div>
    </details>
  );
}
export function MaintenanceResolution({
  binding,
  row,
  busy,
  onResolve,
}: ViewProps & { busy: boolean; onResolve: (success: MaintenanceSuccess) => void }) {
  const { t } = useTranslation();
  const page = useMaintenancePage("successes", binding, row.id, row.revision);
  const [selected, setSelected] = useState("");
  const success = page.entries.find((item) => item.id === selected);
  return (
    <section
      className="space-y-2 rounded-md border p-3 min-w-0"
      aria-label={t("orchestration:maintenanceResolution")}
    >
      <h4 className="font-medium text-sm">{t("orchestration:maintenanceResolution")}</h4>
      <p className="text-xs text-muted-foreground">
        {t("orchestration:maintenanceResolutionHint")}
      </p>
      {Boolean(page.error) && <p role="alert">{t("orchestration:assistantUnavailable")}</p>}
      {page.loaded && !page.entries.length && (
        <p className="text-sm">{t("orchestration:maintenanceNoSuccess")}</p>
      )}
      <MaintenanceSelect
        label={t("orchestration:maintenanceSuccessfulTask")}
        value={selected}
        options={page.entries.map((item) => ({
          id: item.id,
          name: `${item.task_title} (${new Date(item.completed_at).toLocaleString()})`,
        }))}
        onChange={setSelected}
      />
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={busy || page.loading}
          className="cursor-pointer max-md:min-h-11"
          onClick={() => void page.refresh()}
        >
          {t("orchestration:maintenanceRefreshResults")}
        </Button>
        {page.nextCursor && (
          <Button
            variant="outline"
            disabled={busy || page.loading}
            className="cursor-pointer max-md:min-h-11"
            onClick={() => void page.loadMore()}
          >
            {t("orchestration:assistantLoadMore")}
          </Button>
        )}
        <Button
          disabled={!success || busy || Boolean(page.error)}
          className="cursor-pointer max-md:min-h-11"
          onClick={() => {
            if (success) onResolve(success);
          }}
        >
          {t("orchestration:maintenanceResolve")}
        </Button>
      </div>
    </section>
  );
}
