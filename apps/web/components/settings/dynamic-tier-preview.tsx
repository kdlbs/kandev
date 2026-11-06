import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useDynamicTierPreview } from "@/hooks/domains/agents/use-dynamic-tier-preview";
import type { AgentProfile, DynamicAgentCandidate } from "@/lib/types/agent-profile";
import { dynamicProfilePayload } from "./dynamic-agent-profile-editor-state";
import { settingsActionClassName } from "./settings-control";
import type { DynamicTierPreview as PreviewResult } from "@/lib/api/domains/dynamic-tier-preview-api";

function PreviewEntryStatus({ entry }: { entry: PreviewResult["considered"][number] }) {
  const { t } = useTranslation();
  if (entry.selected) return <span>{t("agents:tierPreviewSelected")}</span>;
  if (entry.eligible) return <span>{t("agents:tierPreviewEligible")}</span>;
  return <span>{t("agents:tierPreviewExcluded", { reason: entry.reason })}</span>;
}

export function DynamicTierPreview({
  profileId,
  version,
  candidates,
  profiles,
}: {
  profileId?: string;
  version: number;
  candidates: DynamicAgentCandidate[];
  profiles: AgentProfile[];
}) {
  const { t } = useTranslation();
  const enabled = useFeature("dynamicProfileTiers");
  const document = dynamicProfilePayload("", true, version, candidates).dynamic;
  const { result, preview } = useDynamicTierPreview(enabled, profileId, document);
  if (!enabled) return null;
  return (
    <section className="space-y-2 rounded-md border p-3" data-testid="dynamic-tier-preview">
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName("cursor-pointer")}
        disabled={result?.status === "loading"}
        onClick={() => void preview()}
      >
        {t("agents:tierPreviewAction")}
      </Button>
      <p className="text-xs text-muted-foreground">{t("agents:tierPreviewHelp")}</p>
      {result?.status === "loading" && <p role="status">{t("common:loading")}</p>}
      {result?.status === "failed" && (
        <p role="alert" className="text-sm text-destructive">
          {t("agents:tierPreviewFailed")}
        </p>
      )}
      {result?.preview && (
        <div className="space-y-2 text-sm" role="status">
          {result.preview.state !== "selected" && (
            <p>
              {t(
                result.preview.state === "no_candidates"
                  ? "agents:tierPreviewEmpty"
                  : "agents:tierPreviewNoEligible",
              )}
            </p>
          )}
          {result.preview.considered.map((entry) => (
            <div
              key={entry.candidate_id}
              className="flex min-w-0 flex-wrap gap-x-3 gap-y-1 rounded border p-2"
            >
              <span className="min-w-0 break-all font-medium">
                {profiles.find((profile) => profile.id === entry.candidate_id)?.name ??
                  entry.candidate_id}
              </span>
              <span>{t("agents:orderedTierNumber", { tier: entry.tier })}</span>
              <code>{entry.mode}</code>
              <code>{entry.cost_class}</code>
              <PreviewEntryStatus entry={entry} />
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
