"use client";

import { useTranslation } from "react-i18next";
import { ScriptCard } from "@/components/settings/profile-edit/script-card";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";
import { executorProfileDiscoveryTarget } from "@/lib/settings-discovery/dynamic-targets";
import type { ExecutorType } from "@/lib/types/http";

type ProfileScripts = {
  isRemote: boolean;
  prepareDescription: string;
  prepareValue: string;
  prepareBaseline: string;
  onPrepareChange: (value: string) => void;
  cleanupValue: string;
  cleanupBaseline: string;
  onCleanupChange: (value: string) => void;
  placeholders: ScriptPlaceholder[];
};

export function ProfileScriptCards({
  executorType,
  profileId,
  scripts,
}: {
  executorType: ExecutorType;
  profileId: string;
  scripts: ProfileScripts;
}) {
  const { t } = useTranslation();
  return (
    <>
      <ScriptCard
        title={t("executors:prepareScript")}
        description={scripts.prepareDescription}
        value={scripts.prepareValue}
        baselineValue={scripts.prepareBaseline}
        onChange={scripts.onPrepareChange}
        height="300px"
        placeholders={scripts.placeholders}
        executorType={executorType}
        discoveryTargetId={executorProfileDiscoveryTarget(profileId, "prepare-script")}
      />
      {scripts.isRemote && (
        <ScriptCard
          title={t("executors:cleanupScript")}
          description={t("executors:runsAfterTheAgentSessionEnds")}
          value={scripts.cleanupValue}
          baselineValue={scripts.cleanupBaseline}
          onChange={scripts.onCleanupChange}
          height="200px"
          placeholders={scripts.placeholders}
          executorType={executorType}
        />
      )}
    </>
  );
}
