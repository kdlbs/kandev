"use client";

import { useTranslation } from "react-i18next";
import { useExecutionCommandPreview } from "@/hooks/domains/session/use-execution-command-preview";
import type { ExecutionSnapshot } from "@/lib/types/execution";
import { CommandDetails } from "./execution-command-details";

export function NextExecutionCommand({ snapshot }: { snapshot: ExecutionSnapshot | null }) {
  const { t } = useTranslation();
  const preview = useExecutionCommandPreview(snapshot?.agent_profile_id ?? "");
  if (preview.isPending) return <p role="status">{t("common:loading")}</p>;
  if (!preview.data) return <p>{t("agents:executionCommandUnavailable")}</p>;
  const changed = (preview.data.wrapper?.digest ?? "") !== (snapshot?.wrapper?.digest ?? "");
  return (
    <div className="space-y-3">
      <dl className="space-y-3">
        <div>
          <dt className="text-muted-foreground">{t("agents:agentCommand")}</dt>
          <dd>
            <CommandDetails command={preview.data.agent} />
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("agents:commandPrefix")}</dt>
          <dd>
            {preview.data.wrapper ? (
              <CommandDetails command={preview.data.wrapper.command} />
            ) : (
              t("agents:executionNoWrapper")
            )}
          </dd>
        </div>
      </dl>
      <p>{t(changed ? "agents:executionWrapperChanged" : "agents:executionWrapperUnchanged")}</p>
    </div>
  );
}
