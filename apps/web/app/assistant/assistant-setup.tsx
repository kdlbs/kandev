import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import Link from "@/components/routing/app-link";
import { CoordinatorSelect } from "@/app/coordinator/coordinator-select";
import { orchestratorsHref } from "@/lib/api/domains/orchestration-api";
import { useAssistantSetup } from "@/hooks/domains/orchestration/use-assistant-setup";
import { assistantFailureKey } from "@/hooks/domains/orchestration/use-assistant-actions";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
export function AssistantSetup({
  binding,
  onSelected,
}: {
  binding: AssistantBinding | null;
  onSelected: () => void;
}) {
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const [workspace, setWorkspace] = useState(binding?.home_workspace_id ?? workspaces[0]?.id ?? "");
  const [selected, setSelected] = useState("");
  const state = useAssistantSetup(workspace, binding, onSelected);
  const chooseWorkspace = (id: string) => {
    setWorkspace(id);
    setSelected("");
  };
  return (
    <section className="p-4 md:p-6 space-y-4 max-w-xl" data-testid="assistant-setup">
      <h1 className="text-xl font-semibold">{t("orchestration:assistantSetup")}</h1>
      <p className="text-sm text-muted-foreground">{t("orchestration:assistantSetupHint")}</p>
      <CoordinatorSelect
        label={t("orchestration:assistantHome")}
        value={workspace || "none"}
        onChange={chooseWorkspace}
        options={workspaces.map((item) => ({ id: item.id, name: item.name }))}
        testId="assistant-workspace"
      />
      {state.error && <p role="alert">{t("orchestration:assistantUnavailable")}</p>}
      {!state.data && !state.error && <p role="status">{t("common:loading")}</p>}
      {state.data && (
        <CoordinatorSelect
          label={t("orchestration:assistantChoose")}
          value={selected || "none"}
          onChange={setSelected}
          options={[
            { id: "none", name: t("orchestration:assistantChoose") },
            ...state.data.orchestrators,
          ]}
          testId="assistant-selector"
        />
      )}
      {Boolean(state.actionError) && (
        <p role="alert">{t(assistantFailureKey(state.actionError))}</p>
      )}
      <Button
        disabled={!selected || selected === "none" || state.busy}
        className="cursor-pointer max-md:min-h-11"
        onClick={() => void state.select(selected)}
      >
        {t("orchestration:assistantUseSelected")}
      </Button>
      {workspace && (
        <Link
          href={orchestratorsHref(workspace)}
          className="flex min-h-11 items-center text-sm underline cursor-pointer"
        >
          {t("orchestration:assistantConfigure")}
        </Link>
      )}
    </section>
  );
}
