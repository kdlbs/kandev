import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import { useAssistantPage } from "@/hooks/domains/orchestration/use-assistant";
import {
  useAssistantActivity,
  useAssistantCredentials,
} from "@/hooks/domains/orchestration/use-assistant-details";
import { AssistantPanel } from "./assistant-panel";
import { AssistantObjectives } from "./assistant-objectives";
import { MemoryPanel } from "./memory-panel";
function AssistantCapabilities({
  binding,
  revision,
}: {
  binding: AssistantBinding;
  revision: number;
}) {
  const { t } = useTranslation();
  const page = useAssistantPage("capabilities", binding, revision);
  return (
    <AssistantPanel
      title={t("orchestration:assistantCapabilities")}
      {...page}
      count={page.entries.length}
      onRefresh={() => void page.refresh()}
      onMore={() => void page.loadMore()}
    >
      <p className="text-xs text-muted-foreground">
        {t("orchestration:assistantCapabilitiesHint")}
      </p>
      <ul className="space-y-2">
        {page.entries.map((item) => (
          <li key={item.id} className="rounded-md border p-3 space-y-1">
            <p className="font-medium text-sm break-words">{item.name}</p>
            <p className="text-xs text-muted-foreground">
              {item.kind} · {item.health}
            </p>
            <p className="text-xs">{t(attachmentLabel(item))}</p>
          </li>
        ))}
      </ul>
    </AssistantPanel>
  );
}
function AssistantCredentials({
  binding,
  revision,
}: {
  binding: AssistantBinding;
  revision: number;
}) {
  const { t } = useTranslation();
  const state = useAssistantCredentials(binding, revision);
  const rows = state.data?.credentials ?? [];
  return (
    <AssistantPanel
      title={t("orchestration:assistantCredentials")}
      loading={!state.data && !state.error}
      loaded={Boolean(state.data)}
      error={state.error}
      count={rows.length}
      nextCursor=""
      onRefresh={state.refresh}
      onMore={() => {}}
    >
      <p className="text-xs text-muted-foreground">{t("orchestration:assistantCredentialsHint")}</p>
      {rows.map((row) => (
        <article key={row.id} className="rounded-md border p-3 space-y-1 text-sm">
          <p className="font-medium">{row.purpose}</p>
          <p>
            {row.account} · {row.environment}
          </p>
          <p>{row.validation.status}</p>
          <Link
            href="/settings/agents"
            className="inline-flex items-center text-xs underline cursor-pointer max-md:min-h-11"
          >
            {t("orchestration:assistantAccountSettings")}
          </Link>
        </article>
      ))}
    </AssistantPanel>
  );
}
function AssistantActivity({ binding, revision }: { binding: AssistantBinding; revision: number }) {
  const { t } = useTranslation();
  const page = useAssistantActivity(binding, revision);
  return (
    <AssistantPanel
      title={t("orchestration:assistantActivity")}
      {...page}
      count={page.entries.length}
      onRefresh={() => void page.refresh()}
      onMore={() => void page.loadMore()}
    >
      <p className="text-xs text-muted-foreground">{t("orchestration:assistantActivityHint")}</p>
      <ol className="space-y-3">
        {page.entries
          .slice()
          .reverse()
          .map((row) => (
            <li key={row.id} className="space-y-1 rounded-md border p-3 text-sm">
              <p className="whitespace-pre-wrap break-words">{row.content}</p>
              <p className="text-xs text-muted-foreground">
                {row.runStatus
                  ? t(`orchestration:assistantRun_${row.runStatus}`)
                  : t(
                      row.authorType === "agent"
                        ? "orchestration:assistantReply"
                        : "orchestration:assistantAccepted",
                    )}
                {row.receiptStatus === "superseded" &&
                  ` · ${t("orchestration:assistantSuperseded")}`}
              </p>
              <time dateTime={row.createdAt} className="text-xs text-muted-foreground">
                {new Date(row.createdAt).toLocaleString()}
              </time>
            </li>
          ))}
      </ol>
    </AssistantPanel>
  );
}
export function AssistantDetails(props: { binding: AssistantBinding; revision: number }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <AssistantObjectives {...props} />
      {[
        { key: "assistantMemory", content: <MemoryPanel {...props} /> },
        { key: "assistantActivity", content: <AssistantActivity {...props} /> },
        { key: "assistantCapabilities", content: <AssistantCapabilities {...props} /> },
        { key: "assistantCredentials", content: <AssistantCredentials {...props} /> },
      ].map((panel) => (
        <details key={panel.key} className="rounded-md border p-3">
          <summary className="cursor-pointer py-1 max-md:min-h-11">
            {t(`orchestration:${panel.key}`)}
          </summary>
          <div className="pt-3">{panel.content}</div>
        </details>
      ))}
    </div>
  );
}

function attachmentLabel(item: { attached: boolean; configured: boolean }) {
  if (item.attached) return "orchestration:assistantAttached";
  return item.configured
    ? "orchestration:assistantConfigured"
    : "orchestration:assistantNotConfigured";
}
