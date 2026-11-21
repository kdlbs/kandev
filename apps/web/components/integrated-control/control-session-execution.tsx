import { useTranslation } from "react-i18next";
import type { TaskSession } from "@/lib/types/http";

function reportedSnapshot(session?: TaskSession): Record<string, unknown> | undefined {
  if (!session?.agent_execution_id) return undefined;
  if (session.route_generation && session.route_state !== "active") return undefined;
  const value = session.metadata?.acp_model_state;
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function reportedModel(session?: TaskSession): string | undefined {
  const snapshot = reportedSnapshot(session);
  if (!session || !snapshot) return undefined;
  const generation = snapshot.settings_source_generation;
  if (typeof generation !== "number" || !Number.isSafeInteger(generation) || generation <= 0)
    return undefined;
  if (
    snapshot.settings_source_execution_id !== session.agent_execution_id ||
    snapshot.config_options_settled !== true
  )
    return undefined;
  const model = snapshot.current_model_id;
  return typeof model === "string" ? model.trim() || undefined : undefined;
}

// Use only selected-session evidence; settings profiles and host account reads
// do not establish the model/provider selected by this execution.
export function ControlSessionExecution({ session }: { session?: TaskSession }) {
  const { t } = useTranslation();
  const unknown = t("common:unknown");
  return (
    <dl className="grid min-w-0 basis-full gap-x-4 gap-y-1 sm:grid-cols-3 [&_dd]:break-all">
      <div>
        <dt className="font-medium">{t("sidebar:integratedControl.selectedExecutionProfile")}</dt>
        <dd data-testid="control-selected-profile">
          {session?.execution_profile_id?.trim() || unknown}
        </dd>
      </div>
      <div>
        <dt className="font-medium">{t("sidebar:integratedControl.reportedProvider")}</dt>
        <dd data-testid="control-reported-provider">{unknown}</dd>
      </div>
      <div>
        <dt className="font-medium">{t("sidebar:integratedControl.reportedModel")}</dt>
        <dd data-testid="control-reported-model">{reportedModel(session) ?? unknown}</dd>
      </div>
    </dl>
  );
}
