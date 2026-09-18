import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { PageShell } from "@/components/page-shell";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAssistant } from "@/hooks/domains/orchestration/use-assistant";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import Link from "@/components/routing/app-link";
import { orchestratorHref } from "@/lib/api/domains/orchestration-api";
import { AssistantSetup } from "./assistant-setup";
import { AssistantChat } from "./assistant-chat";
import { AssistantControls } from "./assistant-controls";
import { AssistantDetails } from "./assistant-details";
import { AssistantAttentionPanel } from "./assistant-attention";
export function AssistantPage() {
  const { t } = useTranslation();
  const enabled = useAppStore((s) => s.features.personalAssistant);
  const owner = useAppStore((s) => s.auth.user?.id);
  return (
    <PageShell title={t("orchestration:assistant")} scroll="none">
      {enabled ? (
        <AssistantLoader key={owner} />
      ) : (
        <p className="p-6">{t("orchestration:assistantDisabled")}</p>
      )}
    </PageShell>
  );
}
function AssistantLoader() {
  const { t } = useTranslation();
  const state = useAssistant();
  const [setup, setSetup] = useState(false);
  if (state.error && !state.binding)
    return (
      <div role="alert" className="p-6 space-y-3">
        <p>{t("orchestration:assistantUnavailable")}</p>
        <Button className="cursor-pointer max-md:min-h-11" onClick={() => void state.refresh()}>
          {t("task:retry")}
        </Button>
      </div>
    );
  if (state.binding === undefined)
    return (
      <p className="p-6" role="status">
        {t("common:loading")}
      </p>
    );
  if (state.binding === null || setup)
    return (
      <AssistantSetup
        binding={state.binding}
        onSelected={() => {
          setSetup(false);
          void state.refresh();
        }}
      />
    );
  const binding = state.binding;
  return (
    <AssistantContent
      unavailable={Boolean(state.error)}
      key={`${binding.owner_user_id}:${binding.id}:${binding.version}:${binding.home_workspace_id}`}
      binding={binding}
      revision={state.revision}
      refresh={() => void state.refresh()}
      setup={() => setSetup(true)}
    />
  );
}
function AssistantTabs({
  tab,
  setTab,
  mobile,
}: {
  tab: string;
  setTab: (value: string) => void;
  mobile: boolean;
}) {
  const { t } = useTranslation();
  const tabs = mobile ? ["chat", "attention", "details"] : ["attention", "details"];
  return (
    <div
      role="tablist"
      aria-label={t("orchestration:assistant")}
      className="flex border-b"
      onKeyDown={(event) => {
        if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
        event.preventDefault();
        let index =
          (tabs.indexOf(tab) + (event.key === "ArrowLeft" ? -1 : 1) + tabs.length) % tabs.length;
        if (event.key === "Home") index = 0;
        if (event.key === "End") index = tabs.length - 1;
        const next = tabs[index];
        setTab(next);
        event.currentTarget.querySelector<HTMLElement>(`#assistant-tab-${next}`)?.focus();
      }}
    >
      {tabs.map((id) => (
        <Button
          id={`assistant-tab-${id}`}
          key={id}
          role="tab"
          aria-selected={tab === id}
          aria-controls={`assistant-panel-${id}`}
          tabIndex={tab === id ? 0 : -1}
          variant={tab === id ? "secondary" : "ghost"}
          className="cursor-pointer flex-1 rounded-none max-md:min-h-11"
          onClick={() => setTab(id)}
        >
          {t(`orchestration:assistantTab_${id}`)}
        </Button>
      ))}
    </div>
  );
}
export function AssistantContent({
  binding,
  revision,
  refresh,
  setup,
  unavailable = false,
}: {
  binding: AssistantBinding;
  revision: number;
  refresh: () => void;
  setup: () => void;
  unavailable?: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [mobileTab, setMobileTab] = useState("chat");
  const [detailTab, setDetailTab] = useState("attention");
  const names = useAppStore((s) => s.workspaces.items);
  const workspace =
    names.find((item) => item.id === binding.home_workspace_id)?.name ??
    t("orchestration:assistantHome");
  const sideTab = isMobile && mobileTab !== "chat" ? mobileTab : detailTab;
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="assistant-page">
      <AssistantHeader
        binding={binding}
        unavailable={unavailable}
        workspace={workspace}
        refresh={refresh}
        setup={setup}
      />
      {isMobile && <AssistantTabs tab={mobileTab} setTab={setMobileTab} mobile />}
      <div className="flex min-h-0 min-w-0 flex-1">
        <div
          id="assistant-panel-chat"
          role={isMobile ? "tabpanel" : undefined}
          aria-labelledby={isMobile ? "assistant-tab-chat" : undefined}
          hidden={isMobile && mobileTab !== "chat"}
          className="flex flex-1 min-h-0 min-w-0 flex-col [&[hidden]]:hidden"
        >
          <AssistantChat
            binding={binding}
            revision={revision}
            unavailable={unavailable}
            onAccepted={refresh}
          />
        </div>
        <aside
          hidden={isMobile && mobileTab === "chat"}
          className="flex flex-1 min-h-0 min-w-0 flex-col md:flex-none md:w-[42%] md:max-w-xl md:border-l [&[hidden]]:hidden"
        >
          {!isMobile && <AssistantTabs tab={detailTab} setTab={setDetailTab} mobile={false} />}
          <div
            id={`assistant-panel-${sideTab}`}
            role="tabpanel"
            aria-labelledby={`assistant-tab-${sideTab}`}
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 pb-[max(1rem,env(safe-area-inset-bottom))]"
          >
            {sideTab === "attention" ? (
              <AssistantAttentionPanel binding={binding} revision={revision} onUpdated={refresh} />
            ) : (
              <AssistantDetails binding={binding} revision={revision} />
            )}
          </div>
        </aside>
      </div>
    </div>
  );
}

function AssistantHeader({
  binding,
  unavailable,
  workspace,
  refresh,
  setup,
}: {
  binding: AssistantBinding;
  unavailable: boolean;
  workspace: string;
  refresh: () => void;
  setup: () => void;
}) {
  const { t } = useTranslation();
  return (
    <header className="border-b px-4 py-3 space-y-2">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium truncate">{workspace}</p>
        <Button variant="ghost" className="cursor-pointer max-md:min-h-11" onClick={setup}>
          {t("orchestration:assistantChange")}
        </Button>
      </div>
      <fieldset disabled={unavailable}>
        <AssistantControls binding={binding} refresh={refresh} />
      </fieldset>
      {unavailable && (
        <p role="alert" className="text-sm">
          {t("orchestration:assistantUnavailable")}{" "}
          <Button variant="ghost" className="cursor-pointer max-md:min-h-11" onClick={refresh}>
            {t("task:retry")}
          </Button>
        </p>
      )}
      {(binding.authority_reason || binding.authority?.unsupported_reason) && !binding.paused && (
        <p role="status" className="text-sm">
          {t("orchestration:assistantProfileUnavailable")}{" "}
          <Link
            href={orchestratorHref(binding.home_workspace_id, binding.orchestrator_id)}
            className="underline cursor-pointer"
          >
            {t("orchestration:assistantConfigure")}
          </Link>
        </p>
      )}
    </header>
  );
}
