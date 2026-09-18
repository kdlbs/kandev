import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import {
  saveWorkspaceGrant,
  revokeWorkspaceGrant,
  forgetWorkspaceHandoffs,
} from "@/lib/api/domains/assistant-workspace-api";
import {
  useWorkspacePage,
  useWorkspaceReceiver,
} from "@/hooks/domains/orchestration/use-assistant-workspaces";
import type { WorkspaceGrant } from "@/lib/api/domains/assistant-workspace-types";
import { AssistantPanel } from "./assistant-panel";
import { WorkspaceGrantForm } from "./workspace-grant-form";
import { WorkspaceAudit } from "./workspace-audit";
export function WorkspaceLinks({
  binding,
  revision,
}: {
  binding: AssistantBinding;
  revision: number;
}) {
  const { t } = useTranslation();
  const state = useWorkspaceLinks(binding, revision);
  const { local, editing, setEditing, busy, error, links, options, receiver, act, refresh } = state;
  return (
    <AssistantPanel
      title={t("orchestration:workspaceLinks")}
      {...links}
      count={links.entries.length}
      onRefresh={refresh}
      onMore={() => void links.loadMore()}
    >
      <p className="text-xs text-muted-foreground">{t("orchestration:workspaceHistoryLimit")}</p>
      {error && (
        <p role="alert" className="text-sm">
          {t("orchestration:workspaceChangeFailed")}
        </p>
      )}
      <div className="space-y-3">
        {links.entries.map((row) => (
          <WorkspaceGrantCard
            key={row.id}
            row={row}
            busy={busy}
            onEdit={() => setEditing(row)}
            onRevoke={() =>
              void act(() => revokeWorkspaceGrant(row.workspace_id, binding.version, row.revision))
            }
            onForget={() =>
              void act(() =>
                forgetWorkspaceHandoffs(row.workspace_id, binding.version, row.revision),
              )
            }
          />
        ))}
      </div>
      {receiver.data && !receiver.error && !options.error && (
        <WorkspaceGrantForm
          key={`${editing?.id ?? "new"}:${editing?.revision ?? 0}:${binding.version}:${receiver.data.receiver.profile_revision}`}
          bindingVersion={binding.version}
          receiver={receiver.data.receiver}
          grant={editing}
          choices={options.entries.filter(
            (row) =>
              editing?.workspace_id === row.id ||
              !links.entries.some((link) => link.workspace_id === row.id),
          )}
          busy={busy}
          onSave={(id, request) => void act(() => saveWorkspaceGrant(id, request))}
        />
      )}
      {Boolean(receiver.error || options.error) && (
        <p role="alert" className="text-sm">
          {t("orchestration:assistantUnavailable")}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {options.nextCursor && (
          <Button
            variant="outline"
            disabled={options.loading}
            className="cursor-pointer max-md:min-h-11"
            onClick={() => void options.loadMore()}
          >
            {t("orchestration:workspaceMoreChoices")}
          </Button>
        )}
        <Button
          variant="outline"
          disabled={busy}
          className="cursor-pointer max-md:min-h-11"
          onClick={refresh}
        >
          {t("orchestration:workspaceRefresh")}
        </Button>
      </div>
      <WorkspaceAudit binding={binding} revision={revision + local} />
    </AssistantPanel>
  );
}
function useWorkspaceLinks(binding: AssistantBinding, revision: number) {
  const [local, setLocal] = useState(0);
  const [editing, setEditing] = useState<WorkspaceGrant>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const links = useWorkspacePage("links", binding, revision + local);
  const options = useWorkspacePage("options", binding, revision + local);
  const receiver = useWorkspaceReceiver(binding, revision + local);
  const act = async (action: () => Promise<unknown>) => {
    if (busy) return;
    setBusy(true);
    setError(false);
    try {
      await action();
      setEditing(undefined);
      setLocal((value) => value + 1);
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  };
  const refresh = () => {
    setEditing(undefined);
    setLocal((value) => value + 1);
  };
  return { local, editing, setEditing, busy, error, links, options, receiver, act, refresh };
}
function WorkspaceGrantCard({
  row,
  busy,
  onEdit,
  onRevoke,
  onForget,
}: {
  row: WorkspaceGrant;
  busy: boolean;
  onEdit: () => void;
  onRevoke: () => void;
  onForget: () => void;
}) {
  const { t } = useTranslation();
  return (
    <article className="rounded-md border p-3 space-y-2 min-w-0" data-testid="workspace-grant-card">
      <h3 className="font-medium break-words">{row.workspace_name || row.workspace_id}</h3>
      <p className="text-sm">
        {row.active
          ? t("orchestration:workspaceActive")
          : t(`orchestration:workspaceReason_${row.reason ?? "workspace_unavailable"}`)}
      </p>
      <p className="text-xs text-muted-foreground break-all">
        {t("orchestration:workspaceProfile", {
          id: row.receiver_profile_id,
          revision: row.revision,
        })}
      </p>
      <ul className="text-xs space-y-1">
        {row.scope.context_exports.map((kind) => (
          <li key={kind}>{t(`orchestration:workspaceExport_${kind}`)}</li>
        ))}
      </ul>
      <p className="text-xs">
        {t(
          row.scope.operations.includes("coordinate")
            ? "orchestration:workspaceCoordinate"
            : "orchestration:workspaceObserveOnly",
        )}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={busy || !row.workspace_name}
          className="cursor-pointer max-md:min-h-11"
          onClick={onEdit}
        >
          {t("orchestration:workspaceReview")}
        </Button>
        <Button
          variant="outline"
          disabled={busy || Boolean(row.revoked_at)}
          className="cursor-pointer max-md:min-h-11"
          onClick={onRevoke}
        >
          {t("orchestration:workspaceRevoke")}
        </Button>
        <Button
          variant="outline"
          disabled={busy}
          className="cursor-pointer max-md:min-h-11"
          onClick={onForget}
        >
          {t("orchestration:workspaceForget")}
        </Button>
      </div>
    </article>
  );
}
