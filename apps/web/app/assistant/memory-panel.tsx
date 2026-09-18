import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { generateUUID } from "@/lib/utils";
import type { AssistantBinding, AssistantMemory } from "@/lib/api/domains/assistant-api";
import { useAssistantPage } from "@/hooks/domains/orchestration/use-assistant";
import {
  useAssistantMemoryActions,
  useAssistantMemorySource,
} from "@/hooks/domains/orchestration/use-assistant-memory";
import { assistantFailureKey } from "@/hooks/domains/orchestration/use-assistant-actions";
import { AssistantPanel } from "./assistant-panel";
import { MemoryEditor } from "./memory-editor";
export function MemoryPanel({
  binding,
  revision,
}: {
  binding: AssistantBinding;
  revision: number;
}) {
  const { t } = useTranslation();
  const page = useAssistantPage("memory", binding, revision);
  const actions = useAssistantMemoryActions(() => void page.refresh());
  const [edit, setEdit] = useState<{ id: string; memory?: AssistantMemory }>();
  const [source, setSource] = useState<string>();
  return (
    <AssistantPanel
      title={t("orchestration:assistantMemory")}
      {...page}
      count={page.entries.length}
      onRefresh={() => void page.refresh()}
      onMore={() => void page.loadMore()}
    >
      <p className="text-xs text-muted-foreground">{t("orchestration:assistantForgetHint")}</p>
      <Button
        variant="outline"
        className="cursor-pointer max-md:min-h-11"
        disabled={Boolean(page.error) || Boolean(edit)}
        onClick={() => setEdit({ id: generateUUID() })}
      >
        {t("orchestration:assistantAddMemory")}
      </Button>
      {Boolean(actions.error) && (
        <p role="alert" className="text-sm">
          {t(assistantFailureKey(actions.error))}
        </p>
      )}
      {edit && (
        <MemoryEditor
          key={edit.id}
          binding={binding}
          memory={edit.memory}
          busy={actions.busy}
          onCancel={() => setEdit(undefined)}
          onSave={async (draft) => {
            if (await actions.save(edit.id, draft)) setEdit(undefined);
          }}
        />
      )}
      <div className="space-y-3">
        {page.entries.map((memory) => (
          <MemoryRow
            key={memory.id}
            memory={memory}
            busy={actions.busy}
            editing={Boolean(edit)}
            onEdit={() => setEdit({ id: memory.id, memory })}
            onForget={() => void actions.forget(memory.id, memory.revision)}
            showSource={source === memory.id}
            onSource={() => setSource(source === memory.id ? undefined : memory.id)}
            owner={binding.owner_user_id}
          />
        ))}
      </div>
    </AssistantPanel>
  );
}

function MemorySource({ id, owner }: { id: string; owner: string }) {
  const { t } = useTranslation();
  const source = useAssistantMemorySource(id, owner);
  if (source.error)
    return (
      <p role="alert" className="text-sm">
        {t("orchestration:assistantSourceUnavailable")}
      </p>
    );
  if (!source.data) return <p role="status">{t("common:loading")}</p>;
  return (
    <div className="text-sm space-y-1">
      <blockquote className="whitespace-pre-wrap break-words border-l-2 pl-3">
        {source.data.body}
      </blockquote>
      <time className="text-xs text-muted-foreground" dateTime={source.data.created_at}>
        {new Date(source.data.created_at).toLocaleString()}
      </time>
      {source.data.truncated && (
        <p className="text-xs">{t("orchestration:assistantSourceExcerpt")}</p>
      )}
    </div>
  );
}

function MemoryRow({
  memory,
  busy,
  editing,
  onEdit,
  onForget,
  showSource,
  onSource,
  owner,
}: {
  memory: AssistantMemory;
  busy: boolean;
  editing: boolean;
  onEdit: () => void;
  onForget: () => void;
  showSource: boolean;
  onSource: () => void;
  owner: string;
}) {
  const { t } = useTranslation();
  return (
    <article
      key={memory.id}
      className="space-y-2 rounded-md border p-3"
      data-testid="assistant-memory-row"
    >
      <h3 className="font-medium">{memory.key}</h3>
      <p className="whitespace-pre-wrap break-words text-sm">{memory.content}</p>
      <p className="text-xs text-muted-foreground">
        {t(`orchestration:assistantScope_${memory.scope}`)} ·{" "}
        {t(
          memory.confirmed
            ? "orchestration:assistantConfirmed"
            : "orchestration:assistantUnconfirmed",
        )}
      </p>
      {memory.expires_at && (
        <p className="text-xs">
          {t("orchestration:assistantExpires", {
            date: new Date(memory.expires_at).toLocaleDateString(),
          })}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="ghost"
          className="cursor-pointer max-md:min-h-11"
          disabled={busy || editing}
          onClick={onEdit}
        >
          {t("common:edit")}
        </Button>
        <Button
          variant="ghost"
          className="cursor-pointer max-md:min-h-11"
          disabled={busy}
          onClick={onForget}
        >
          {t("orchestration:assistantForget")}
        </Button>
        {memory.source_comment_id && (
          <Button variant="ghost" className="cursor-pointer max-md:min-h-11" onClick={onSource}>
            {t("orchestration:assistantMemorySource")}
          </Button>
        )}
      </div>
      {showSource && <MemorySource id={memory.id} owner={owner} />}
    </article>
  );
}
