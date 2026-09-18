import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type {
  WorkspaceExportKind,
  WorkspaceGrant,
  WorkspaceGrantRequest,
  WorkspaceGrantScope,
  WorkspaceReceiver,
} from "@/lib/api/domains/assistant-workspace-types";
export type WorkspaceGrantFormProps = {
  bindingVersion: number;
  receiver: WorkspaceReceiver;
  grant?: WorkspaceGrant;
  choices: { id: string; name: string }[];
  busy: boolean;
  onSave: (id: string, request: WorkspaceGrantRequest) => void;
};
export function WorkspaceGrantForm(props: WorkspaceGrantFormProps) {
  const { t } = useTranslation();
  const { bindingVersion, receiver, grant, choices, busy, onSave } = props;
  const [workspace, setWorkspace] = useState(grant?.workspace_id ?? "");
  const [scope, setScope] = useState<WorkspaceGrantScope>(
    grant?.scope ?? { operations: ["observe"], context_exports: ["task_summary"] },
  );
  const [confirmed, setConfirmed] = useState("");
  const request = {
    expected_binding_version: bindingVersion,
    expected_revision: grant?.revision ?? 0,
    receiver,
    scope,
  };
  const identity = JSON.stringify({ workspace, request });
  const allowed =
    confirmed === identity &&
    choices.some((row) => row.id === workspace) &&
    scope.context_exports.length > 0;
  const toggle = (kind: WorkspaceExportKind, checked: boolean) =>
    setScope((old) => ({
      ...old,
      context_exports: toggleExport(old.context_exports, kind, checked),
    }));
  return (
    <form
      className="space-y-3 rounded-md border p-3 min-w-0"
      data-testid="workspace-grant-form"
      onSubmit={(event) => {
        event.preventDefault();
        if (allowed && !busy) onSave(workspace, request);
      }}
    >
      <p className="text-sm font-medium break-words">
        {t("orchestration:workspaceReceiver", {
          name: receiver.profile_name,
          id: receiver.profile_id,
        })}
      </p>
      <p className="text-xs text-muted-foreground">{t("orchestration:workspaceLinkHint")}</p>
      <fieldset disabled={busy} className="space-y-3 min-w-0">
        <label className="block text-sm space-y-1">
          <span>{t("orchestration:workspaceLabel")}</span>
          <select
            className="w-full min-w-0 rounded-md border bg-background p-2 max-md:min-h-11"
            value={workspace}
            disabled={Boolean(grant)}
            onChange={(event) => setWorkspace(event.target.value)}
          >
            <option value="">{t("orchestration:workspaceChoose")}</option>
            {choices.map((row) => (
              <option key={row.id} value={row.id}>
                {row.name}
              </option>
            ))}
          </select>
        </label>
        <WorkspaceCheck
          checked={scope.operations.includes("coordinate")}
          label={t("orchestration:workspaceCoordinate")}
          onChange={(checked) =>
            setScope((old) => ({
              ...old,
              operations: checked ? ["observe", "coordinate"] : ["observe"],
            }))
          }
        />
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium mb-2">
            {t("orchestration:workspaceExportFields")}
          </legend>
          {exportKinds.map((kind) => (
            <WorkspaceCheck
              key={kind}
              checked={scope.context_exports.includes(kind)}
              label={t(`orchestration:workspaceExport_${kind}`)}
              onChange={(checked) => toggle(kind, checked)}
            />
          ))}
        </fieldset>
        <WorkspaceCheck
          checked={confirmed === identity}
          label={t("orchestration:workspaceConfirmReceiver")}
          onChange={(checked) => setConfirmed(checked ? identity : "")}
        />
        <Button type="submit" disabled={!allowed} className="cursor-pointer max-md:min-h-11">
          {t("orchestration:workspaceConfirmAccess")}
        </Button>
      </fieldset>
    </form>
  );
}
const exportKinds: WorkspaceExportKind[] = [
  "directory",
  "task_summary",
  "task_result",
  "task_input",
  "handoff",
];
function toggleExport(values: WorkspaceExportKind[], kind: WorkspaceExportKind, checked: boolean) {
  const next = new Set(values);
  if (checked) next.add(kind);
  else next.delete(kind);
  if (checked && (kind === "task_result" || kind === "task_input")) next.add("task_summary");
  if (!checked && kind === "task_summary") {
    next.delete("task_result");
    next.delete("task_input");
  }
  return [...next];
}
function WorkspaceCheck({
  checked,
  label,
  onChange,
}: {
  checked: boolean;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-2 text-sm cursor-pointer py-1 max-md:min-h-11">
      <input
        type="checkbox"
        className="mt-1 size-4 shrink-0"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span>{label}</span>
    </label>
  );
}
