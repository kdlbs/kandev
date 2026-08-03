"use client";

import { useCallback } from "react";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import {
  LabelMultiSelect,
  PriorityMultiSelect,
  StateMultiSelect,
  useTeamsAndStates,
} from "./linear-issue-watch-fields";
import {
  ASSIGNED_ANY,
  CREATOR_ANY,
  type FormState,
  type LinearPriority,
  creatorPlaceholder,
  userOptionLabel,
} from "./linear-issue-watch-form";
import type { LinearTeam, LinearUser } from "@/lib/types/linear";

// The filter half of the watcher dialog — the "which issues match?" block.
// Split out of linear-issue-watch-dialog.tsx so that file stays under the
// 600-line ceiling once every string routes through `t()`.

export type SelectFieldItem = { id: string; label: string; icon?: React.ReactNode };

export function SelectField(props: {
  label: string;
  description?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  items: SelectFieldItem[];
  disabled?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{props.label}</Label>
      {props.description && <p className="text-xs text-muted-foreground">{props.description}</p>}
      <Select
        value={props.value || undefined}
        onValueChange={props.onChange}
        disabled={props.disabled}
      >
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={props.placeholder} />
        </SelectTrigger>
        <SelectContent>
          {props.items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.icon ? (
                <span className="flex items-center gap-1.5">
                  <span>{item.label}</span>
                  {item.icon}
                </span>
              ) : (
                item.label
              )}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

type FormSetter = React.Dispatch<React.SetStateAction<FormState>>;

export function FilterFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  const { teams, states, labels, users, loadingStates, loadingLabels, loadingUsers } =
    useTeamsAndStates(form.workspaceId, form.teamKey);
  const toggleState = useCallback(
    (id: string) =>
      setForm((p) => ({
        ...p,
        stateIds: p.stateIds.includes(id)
          ? p.stateIds.filter((s) => s !== id)
          : [...p.stateIds, id],
      })),
    [setForm],
  );
  const toggleLabel = useCallback(
    (id: string) =>
      setForm((p) => ({
        ...p,
        labelIds: p.labelIds.includes(id)
          ? p.labelIds.filter((l) => l !== id)
          : [...p.labelIds, id],
      })),
    [setForm],
  );
  const togglePriority = useCallback(
    (priority: LinearPriority) =>
      setForm((p) => ({
        ...p,
        priorities: p.priorities.includes(priority)
          ? p.priorities.filter((x) => x !== priority)
          : [...p.priorities, priority],
      })),
    [setForm],
  );

  return (
    <>
      <TeamRow form={form} setForm={setForm} teams={teams} />
      <AssigneeAndCreatorRow
        form={form}
        setForm={setForm}
        users={users}
        loadingUsers={loadingUsers}
      />
      <div className="space-y-1.5">
        <Label>{t("linear:priority")}</Label>
        <p className="text-xs text-muted-foreground">{t("linear:priorityHelp")}</p>
        <PriorityMultiSelect selected={form.priorities} onToggle={togglePriority} />
      </div>
      <div className="space-y-1.5">
        <Label>{t("linear:states")}</Label>
        <p className="text-xs text-muted-foreground">
          {form.teamKey ? t("linear:statesHelp") : t("linear:statesHelpNoTeam")}
        </p>
        <StateMultiSelect
          states={states}
          loading={loadingStates}
          selected={form.stateIds}
          onToggle={toggleState}
          disabled={!form.teamKey}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("linear:labels")}</Label>
        <p className="text-xs text-muted-foreground">
          {form.teamKey ? t("linear:labelsHelp") : t("linear:labelsHelpNoTeam")}
        </p>
        <LabelMultiSelect
          labels={labels}
          loading={loadingLabels}
          selected={form.labelIds}
          onToggle={toggleLabel}
          disabled={!form.teamKey}
        />
      </div>
      <EstimateRow form={form} setForm={setForm} />
      <QueryField form={form} setForm={setForm} />
    </>
  );
}

function TeamRow({
  form,
  setForm,
  teams,
}: {
  form: FormState;
  setForm: FormSetter;
  teams: LinearTeam[];
}) {
  const { t } = useTranslation();
  return (
    <SelectField
      label={t("linear:team")}
      description={t("linear:teamHelp")}
      value={form.teamKey}
      onChange={(v) =>
        setForm((p) => ({ ...p, teamKey: v, stateIds: [], labelIds: [], creatorId: "" }))
      }
      placeholder={t("linear:anyTeam")}
      // Team names and keys come from the Linear API — user data, never
      // translated, so they travel as a plain `label`.
      items={teams.map((team) => ({ id: team.key, label: `${team.name} (${team.key})` }))}
    />
  );
}

function AssigneeAndCreatorRow({
  form,
  setForm,
  users,
  loadingUsers,
}: {
  form: FormState;
  setForm: FormSetter;
  users: LinearUser[];
  loadingUsers: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-4">
      <SelectField
        label={t("linear:assignee")}
        description={t("linear:assigneeHelp")}
        value={form.assigned || ASSIGNED_ANY}
        onChange={(v) => setForm((p) => ({ ...p, assigned: v === ASSIGNED_ANY ? "" : v }))}
        placeholder={t("linear:any")}
        // `id` values are sentinels the filter payload is built from — "me" and
        // "unassigned" are sent to the backend verbatim and must never be
        // translated. Only the labels are copy.
        items={[
          { id: ASSIGNED_ANY, label: t("linear:any") },
          { id: "me", label: t("linear:me") },
          { id: "unassigned", label: t("linear:unassigned") },
        ]}
      />
      <SelectField
        label={t("linear:creator")}
        description={t("linear:creatorHelp")}
        value={form.creatorId || CREATOR_ANY}
        onChange={(v) => setForm((p) => ({ ...p, creatorId: v === CREATOR_ANY ? "" : v }))}
        placeholder={creatorPlaceholder(t, form.teamKey, loadingUsers)}
        items={[
          { id: CREATOR_ANY, label: t("linear:any") },
          ...users.map((u) => ({ id: u.id, label: userOptionLabel(u) })),
        ]}
        disabled={!form.teamKey || loadingUsers}
      />
    </div>
  );
}

function EstimateRow({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="space-y-1.5">
        <Label>{t("linear:estimateMin")}</Label>
        <p className="text-xs text-muted-foreground">{t("linear:estimateMinHelp")}</p>
        <Input
          type="number"
          value={form.estimateMin}
          onChange={(e) => setForm((p) => ({ ...p, estimateMin: e.target.value }))}
          min={0}
          step="0.5"
          placeholder={t("linear:estimateMinPlaceholder")}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("linear:estimateMax")}</Label>
        <p className="text-xs text-muted-foreground">{t("linear:estimateMaxHelp")}</p>
        <Input
          type="number"
          value={form.estimateMax}
          onChange={(e) => setForm((p) => ({ ...p, estimateMax: e.target.value }))}
          min={0}
          step="0.5"
          placeholder={t("linear:estimateMaxPlaceholder")}
        />
      </div>
    </div>
  );
}

function QueryField({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label>{t("linear:query")}</Label>
      <p className="text-xs text-muted-foreground">{t("linear:queryHelp")}</p>
      <Input
        value={form.query}
        onChange={(e) => setForm((p) => ({ ...p, query: e.target.value }))}
        placeholder={t("linear:queryPlaceholder")}
      />
    </div>
  );
}
