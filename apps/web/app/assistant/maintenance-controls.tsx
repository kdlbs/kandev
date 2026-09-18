import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { useMaintenanceMutations } from "@/hooks/domains/orchestration/use-maintenance-mutations";
import {
  reconcileMaintenance,
  reviewImprovement,
  revokeMaintenanceGrant,
} from "@/lib/api/domains/assistant-maintenance-api";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type {
  ImprovementDetail,
  MaintenanceAction,
} from "@/lib/api/domains/assistant-maintenance-types";
type Props = {
  binding: AssistantBinding;
  detail: ImprovementDetail;
  action: ReturnType<typeof useMaintenanceMutations>;
};
export function MaintenanceGrantCard({
  binding,
  detail,
  action,
  current,
}: Props & { current: boolean }) {
  const { t } = useTranslation();
  const row = detail.candidate,
    grant = detail.grant;
  if (!grant) return null;
  return (
    <div className="rounded-md bg-muted/40 p-3 text-sm space-y-2">
      <p>
        {t(
          current
            ? "orchestration:maintenanceGrantActive"
            : "orchestration:maintenanceGrantInactive",
        )}
      </p>
      <p>
        {t("orchestration:maintenanceExpires", {
          date: new Date(grant.expires_at).toLocaleString(),
        })}
      </p>
      <ul className="list-disc pl-5 break-all">
        {grant.scope.files.map((path) => (
          <li key={path}>
            <code>{path}</code>
          </li>
        ))}
      </ul>
      <p className="text-xs">{t("orchestration:maintenanceGrantHint")}</p>
      {!grant.revoked_at && (
        <Button
          variant="outline"
          disabled={action.busy}
          className="cursor-pointer max-md:min-h-11"
          onClick={() =>
            void action.perform(() =>
              revokeMaintenanceGrant(row.id, binding.version, grant.revision),
            )
          }
        >
          {t("orchestration:maintenanceRevoke")}
        </Button>
      )}
    </div>
  );
}
export function MaintenanceControls({
  binding,
  detail,
  action,
  mutable,
  onArtifact,
}: Props & { mutable: boolean; onArtifact: () => void }) {
  const { t } = useTranslation();
  const row = detail.candidate;
  const run = (kind: MaintenanceAction) => void action.maintenance(kind);
  return (
    <div className="flex flex-wrap gap-2">
      {row.state === "proposed" && (
        <Button
          disabled={!mutable || action.busy}
          className="cursor-pointer max-md:min-h-11"
          onClick={() => run("prepare")}
        >
          {t("orchestration:maintenancePrepare")}
        </Button>
      )}
      {row.state === "investigating" && (
        <>
          <Button
            disabled={!mutable || action.busy}
            variant="outline"
            className="cursor-pointer max-md:min-h-11"
            onClick={() => run("check")}
          >
            {t("orchestration:maintenanceRunChecks")}
          </Button>
          <Button
            disabled={!mutable || action.busy || !detail.validation?.passed}
            className="cursor-pointer max-md:min-h-11"
            onClick={() => run("commit")}
          >
            {t("orchestration:maintenanceCommit")}
          </Button>
        </>
      )}
      {row.repair_task_id && (
        <Button variant="outline" className="cursor-pointer max-md:min-h-11" onClick={onArtifact}>
          {t("orchestration:maintenanceArtifact")}
        </Button>
      )}
      {row.state === "unknown" && (
        <Button
          variant="outline"
          disabled={action.busy}
          className="cursor-pointer max-md:min-h-11"
          onClick={() =>
            void action.perform(() => reconcileMaintenance(row.id, binding.version, row.revision))
          }
        >
          {t("orchestration:maintenanceReconcile")}
        </Button>
      )}
      {!["rejected", "resolved"].includes(row.state) && (
        <Button
          variant="outline"
          disabled={action.busy}
          className="cursor-pointer max-md:min-h-11"
          onClick={() =>
            void action.perform(() =>
              reviewImprovement(row.id, binding.version, row.revision, "rejected"),
            )
          }
        >
          {t("orchestration:maintenanceReject")}
        </Button>
      )}
    </div>
  );
}
