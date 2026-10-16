import { CardDescription } from "@kandev/ui/card";
import { useTranslation } from "react-i18next";
import type { RunsWith } from "@/lib/api/domains/coordinator-runs-with";

export function RunsWithLine({ runsWith }: { runsWith: RunsWith | null | undefined }) {
  const { t } = useTranslation();
  if (!runsWith) return null;
  const text =
    runsWith.source === "none"
      ? t("coordinator:proposalNoAgentAvailable")
      : t("coordinator:proposalRunsWith", {
          name: runsWith.agent_profile_name || runsWith.agent_profile_id,
        });
  return <CardDescription data-testid="proposal-runs-with">{text}</CardDescription>;
}
