import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useMaintenanceMutations } from "@/hooks/domains/orchestration/use-maintenance-mutations";
import { assistantFailureKey } from "@/hooks/domains/orchestration/use-assistant-actions";
import {
  reviewImprovement,
  saveMaintenanceGrant,
} from "@/lib/api/domains/assistant-maintenance-api";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type {
  ImprovementDetail,
  MaintenanceValidation,
} from "@/lib/api/domains/assistant-maintenance-types";
import { MaintenanceControls, MaintenanceGrantCard } from "./maintenance-controls";
import { MaintenanceGrantForm } from "./maintenance-grant-form";
import {
  MaintenanceArtifactView,
  MaintenanceEvidence,
  MaintenanceFileView,
  MaintenanceResolution,
} from "./maintenance-results";
function maintenanceAuthority(binding: AssistantBinding, grant: ImprovementDetail["grant"]) {
  const current = Boolean(
    grant &&
    !grant.revoked_at &&
    grant.binding_version === binding.version &&
    new Date(grant.expires_at).getTime() > Date.now(),
  );
  const mutable =
    current &&
    binding.execution_mode === "execute" &&
    !binding.paused &&
    !binding.authority_reason &&
    !binding.authority?.unsupported_reason;
  return { current, mutable };
}
export function MaintenanceReview({
  binding,
  detail,
  onUpdated,
}: {
  binding: AssistantBinding;
  detail: ImprovementDetail;
  onUpdated: () => void;
}) {
  const { t } = useTranslation();
  const action = useMaintenanceMutations(binding, detail, onUpdated);
  const [artifact, setArtifact] = useState(false);
  const row = detail.candidate,
    grant = detail.grant;
  const { current, mutable } = maintenanceAuthority(binding, grant);
  return (
    <div className="space-y-4 min-w-0">
      <MaintenanceEvidence binding={binding} row={row} />
      {grant && (
        <MaintenanceGrantCard binding={binding} detail={detail} action={action} current={current} />
      )}
      {row.state === "proposed" && (
        <MaintenanceGrantForm
          key={grant?.revision ?? 0}
          binding={binding}
          detail={detail}
          busy={action.busy}
          onSave={(request) => void action.perform(() => saveMaintenanceGrant(row.id, request))}
        />
      )}
      {Boolean(action.error) && (
        <p role="alert" className="text-sm">
          {t(assistantFailureKey(action.error))}
        </p>
      )}
      {current && !mutable && (
        <p className="text-sm">{t("orchestration:maintenanceExecuteRequired")}</p>
      )}
      <MaintenanceControls
        binding={binding}
        detail={detail}
        action={action}
        mutable={mutable}
        onArtifact={() => setArtifact((value) => !value)}
      />
      <MaintenanceReviewResults
        binding={binding}
        detail={detail}
        action={action}
        current={current}
        artifact={artifact}
      />
    </div>
  );
}
function MaintenanceReviewResults({
  binding,
  detail,
  action,
  current,
  artifact,
}: {
  binding: AssistantBinding;
  detail: ImprovementDetail;
  action: ReturnType<typeof useMaintenanceMutations>;
  current: boolean;
  artifact: boolean;
}) {
  const { t } = useTranslation();
  const row = detail.candidate,
    grant = detail.grant;
  return (
    <>
      {row.state === "investigating" && (
        <p className="text-sm">{t("orchestration:maintenanceAskAssistant")}</p>
      )}
      {row.state === "unknown" && (
        <p role="status" className="text-sm">
          {t("orchestration:maintenanceUnknown")}
        </p>
      )}
      {row.repair_task_id && current && grant && (
        <MaintenanceFileView binding={binding} row={row} grant={grant} />
      )}
      {detail.validation && <MaintenanceChecks validation={detail.validation} />}
      {artifact && <MaintenanceArtifactView binding={binding} row={row} />}
      {row.state === "prepared" && (
        <MaintenanceResolution
          binding={binding}
          row={row}
          busy={action.busy}
          onResolve={(success) =>
            void action.perform(() =>
              reviewImprovement(row.id, binding.version, row.revision, "resolved", success),
            )
          }
        />
      )}
      {detail.review && (
        <p className="text-xs text-muted-foreground">
          {t("orchestration:maintenanceReviewed", {
            date: new Date(detail.review.created_at).toLocaleString(),
          })}
        </p>
      )}
    </>
  );
}
export function MaintenanceChecks({ validation }: { validation: MaintenanceValidation }) {
  const { t } = useTranslation();
  return (
    <section className="space-y-2" aria-label={t("orchestration:maintenanceChecks")}>
      <h4 className="text-sm font-medium">
        {t(
          validation.passed
            ? "orchestration:maintenanceChecksPassed"
            : "orchestration:maintenanceChecksFailed",
        )}
      </h4>
      {validation.checks.map((check) => (
        <details key={check.kind} className="rounded-md border p-2">
          <summary className="cursor-pointer text-sm max-md:min-h-11">
            {t(`orchestration:maintenanceCheck_${check.kind}`)} ·{" "}
            {t("orchestration:maintenanceExit", { code: check.exit_code })}
          </summary>
          <pre className="text-xs whitespace-pre-wrap break-all max-h-48 overflow-y-auto">
            {check.summary}
          </pre>
        </details>
      ))}
    </section>
  );
}
