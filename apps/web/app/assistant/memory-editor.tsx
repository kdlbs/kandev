import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Label } from "@kandev/ui/label";
import { Checkbox } from "@kandev/ui/checkbox";
import type {
  AssistantBinding,
  AssistantMemory,
  AssistantMemoryEdit,
} from "@/lib/api/domains/assistant-api";
import { CoordinatorSelect } from "@/app/coordinator/coordinator-select";
import { useAssistantMemorySources } from "@/hooks/domains/orchestration/use-assistant-memory";
export function MemoryEditor({
  binding,
  memory,
  busy,
  onSave,
  onCancel,
}: {
  binding: AssistantBinding;
  memory?: AssistantMemory;
  busy: boolean;
  onSave: (draft: AssistantMemoryEdit) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const sources = useAssistantMemorySources(binding);
  const [draft, setDraft] = useState<AssistantMemoryEdit>(() => memoryDraft(binding, memory));
  const update = <K extends keyof AssistantMemoryEdit>(key: K, value: AssistantMemoryEdit[K]) =>
    setDraft((old) => ({ ...old, [key]: value }));
  return (
    <form
      className="space-y-3 rounded-md border p-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSave(draft);
      }}
    >
      <MemoryFields binding={binding} draft={draft} update={update} />
      <MemoryProvenance draft={draft} update={update} sources={sources} />
      <p className="text-xs text-muted-foreground">{t("orchestration:assistantMemorySafety")}</p>
      <div className="flex gap-2">
        <Button
          type="submit"
          disabled={busy || !draft.source_comment_id}
          className="cursor-pointer max-md:min-h-11"
        >
          {t("common:save")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          className="cursor-pointer max-md:min-h-11"
          onClick={onCancel}
        >
          {t("common:cancel")}
        </Button>
      </div>
    </form>
  );
}

type MemoryFieldsProps = {
  draft: AssistantMemoryEdit;
  update: <K extends keyof AssistantMemoryEdit>(key: K, value: AssistantMemoryEdit[K]) => void;
};
function memoryDraft(binding: AssistantBinding, memory?: AssistantMemory): AssistantMemoryEdit {
  if (!memory)
    return {
      key: "",
      content: "",
      scope: "workspace",
      scope_id: binding.home_workspace_id,
      source_comment_id: "",
      confirmed: false,
      expected_revision: 0,
      priority: 0,
      expires_at: null,
    };
  const {
    key,
    content,
    scope,
    scope_id,
    source_comment_id,
    confirmed,
    priority,
    expires_at,
    revision,
  } = memory;
  return {
    key,
    content,
    scope,
    scope_id,
    source_comment_id,
    confirmed,
    priority,
    expires_at,
    expected_revision: revision,
  };
}
function updateScope(
  value: string,
  binding: AssistantBinding,
  update: MemoryFieldsProps["update"],
) {
  let id = "";
  if (value === "workspace") id = binding.home_workspace_id;
  if (value === "user") id = binding.owner_user_id;
  update("scope", value as AssistantMemory["scope"]);
  update("scope_id", id);
}
function expiryAtEndOfDay(date: string) {
  return date ? `${date}T23:59:59Z` : null;
}
function MemoryFields({
  binding,
  draft,
  update,
}: MemoryFieldsProps & { binding: AssistantBinding }) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-1">
        <Label htmlFor="assistant-memory-key">{t("orchestration:assistantMemoryName")}</Label>
        <Input
          id="assistant-memory-key"
          required
          maxLength={200}
          value={draft.key}
          onChange={(event) => update("key", event.target.value)}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="assistant-memory-content">
          {t("orchestration:assistantMemoryContent")}
        </Label>
        <Textarea
          id="assistant-memory-content"
          required
          value={draft.content}
          onChange={(event) => update("content", event.target.value)}
        />
      </div>
      <CoordinatorSelect
        label={t("orchestration:assistantMemoryScope")}
        value={draft.scope}
        onChange={(value) => updateScope(value, binding, update)}
        options={["user", "workspace", "project", "task", "environment"].map((id) => ({
          id,
          name: t(`orchestration:assistantScope_${id}`),
        }))}
        testId="assistant-memory-scope"
      />
      {!["user", "workspace"].includes(draft.scope) && (
        <div className="space-y-1">
          <Label htmlFor="assistant-memory-scope-id">
            {t("orchestration:assistantMemoryScopeId")}
          </Label>
          <Input
            id="assistant-memory-scope-id"
            required
            value={draft.scope_id}
            onChange={(event) => update("scope_id", event.target.value)}
          />
        </div>
      )}
    </>
  );
}
function MemoryProvenance({
  draft,
  update,
  sources,
}: MemoryFieldsProps & { sources: ReturnType<typeof useAssistantMemorySources> }) {
  const { t } = useTranslation();
  const sourceOptions = sources.comments.map((comment) => ({
    id: comment.id,
    name: comment.content.slice(0, 90),
  }));
  if (
    draft.source_comment_id &&
    !sourceOptions.some((option) => option.id === draft.source_comment_id)
  )
    sourceOptions.push({
      id: draft.source_comment_id,
      name: t("orchestration:assistantOriginalSource"),
    });
  return (
    <>
      <CoordinatorSelect
        label={t("orchestration:assistantMemorySource")}
        value={draft.source_comment_id || "none"}
        onChange={(value) => update("source_comment_id", value === "none" ? "" : value)}
        options={[{ id: "none", name: t("orchestration:assistantChooseSource") }, ...sourceOptions]}
        testId="assistant-memory-source"
      />
      {sources.error && (
        <p role="alert" className="text-sm">
          {t("orchestration:assistantSourceUnavailable")}
        </p>
      )}
      <div className="flex items-center gap-2">
        <Checkbox
          id="assistant-memory-confirmed"
          checked={draft.confirmed}
          onCheckedChange={(value) => update("confirmed", value === true)}
        />
        <Label
          htmlFor="assistant-memory-confirmed"
          className="cursor-pointer max-md:min-h-11 inline-flex items-center"
        >
          {t("orchestration:assistantMemoryConfirmed")}
        </Label>
      </div>
      <div className="space-y-1">
        <Label htmlFor="assistant-memory-expiry">{t("orchestration:assistantMemoryExpiry")}</Label>
        <Input
          id="assistant-memory-expiry"
          type="date"
          value={draft.expires_at?.slice(0, 10) ?? ""}
          onChange={(event) => update("expires_at", expiryAtEndOfDay(event.target.value))}
        />
      </div>
    </>
  );
}
