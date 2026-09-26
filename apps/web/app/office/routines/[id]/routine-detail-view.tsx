"use client";

import { useCallback, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { IconPlayerPlay, IconDeviceFloppy } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { updateRoutine, runRoutine } from "@/lib/api/domains/office-api";
import type {
  Routine,
  RoutineStatus,
  RoutineTrigger,
  UpdateRoutinePatch,
} from "@/lib/state/slices/office/types";
import { timeAgo } from "@/lib/utils/time";
import { useOfficeTopbar } from "../../components/office-topbar-context";
import { isRoutineFiring } from "../../lib/routine-status";
import { routineNotFiringMessage } from "../../lib/routine-not-firing";
import { ScheduleStateBadge, UnarmedScheduleHint } from "../schedule-state-badge";
import { coerceCatchUpMax } from "../../lib/catch-up-max";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { reconcileCronTrigger, type CronReconcileOutcome } from "./cron-reconcile";

// Lift the form state out of the component so the file stays under the
// 100-line per-function ceiling and the helpers can render typed slices
// of the draft without re-deriving every field on each call.
type DraftState = {
  name: string;
  description: string;
  // Undefined means the stored status is not one of the three selectable
  // options; the control shows no selection and a save omits the field.
  status: RoutineStatus | undefined;
  assigneeAgentProfileId: string;
  concurrencyPolicy: string;
  catchUpPolicy: string;
  catchUpMax: string;
  triggerKind: "cron" | "webhook";
  cronExpression: string;
  timezone: string;
};

const ROUTINE_STATUSES: RoutineStatus[] = ["active", "paused", "archived"];

function normalizeDraftStatus(status: string): RoutineStatus | undefined {
  return (ROUTINE_STATUSES as string[]).includes(status) ? (status as RoutineStatus) : undefined;
}

function pickTriggerKind(triggers: RoutineTrigger[]): "cron" | "webhook" {
  const cron = triggers.find((t) => t.kind === "cron");
  if (cron) return "cron";
  const webhook = triggers.find((t) => t.kind === "webhook");
  if (webhook) return "webhook";
  return "cron";
}

function buildDraft(routine: Routine, triggers: RoutineTrigger[]): DraftState {
  const cron = triggers.find((t) => t.kind === "cron");
  const triggerKind = pickTriggerKind(triggers);
  return {
    name: routine.name,
    description: routine.description ?? "",
    status: normalizeDraftStatus(routine.status),
    assigneeAgentProfileId: routine.assigneeAgentProfileId ?? "",
    concurrencyPolicy: routine.concurrencyPolicy ?? "coalesce_if_active",
    catchUpPolicy: routine.catchUpPolicy ?? "summarize_missed",
    catchUpMax: String(coerceCatchUpMax(routine.catchUpMax)),
    triggerKind,
    cronExpression: cron?.cronExpression ?? "",
    timezone: cron?.timezone ?? "UTC",
  };
}

function buildUpdatePatch(draft: DraftState): UpdateRoutinePatch {
  return {
    name: draft.name,
    description: draft.description,
    status: draft.status,
    assigneeAgentProfileId: draft.assigneeAgentProfileId,
    concurrencyPolicy: draft.concurrencyPolicy,
    catchUpPolicy: draft.catchUpPolicy,
    catchUpMax: coerceCatchUpMax(draft.catchUpMax),
  };
}

function describeCronOutcome(
  outcome: CronReconcileOutcome,
  t: TFunction,
): { toastKind: "success" | "error"; message: string; refresh: boolean } {
  if (outcome.kind === "unchanged") {
    return { toastKind: "success", message: t("office:routineSaved"), refresh: true };
  }
  if (outcome.kind === "success") {
    return outcome.triggers === null
      ? {
          toastKind: "success",
          message: t("office:routineSavedScheduleMayBeStale"),
          refresh: false,
        }
      : { toastKind: "success", message: t("office:routineSaved"), refresh: true };
  }
  if (outcome.triggers === null) {
    return { toastKind: "error", message: t("office:routineScheduleFateUnknown"), refresh: false };
  }
  if (outcome.kind === "create-failed") {
    return {
      toastKind: "error",
      message: t("office:routineSavedScheduleChangeFailed", { error: outcome.message }),
      refresh: false,
    };
  }
  const count = outcome.triggers.filter((trigger) => trigger.kind === "cron").length;
  return {
    toastKind: "error",
    message: t("office:routineSavedScheduleDeleteFailed", { error: outcome.message, count }),
    refresh: false,
  };
}

type RoutineDetailViewProps = {
  initialRoutine: Routine;
  initialTriggers: RoutineTrigger[];
};

export function RoutineDetailView({ initialRoutine, initialTriggers }: RoutineDetailViewProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const agents = useAppStore(selectOfficeAgentProfiles);
  const [routine] = useState(initialRoutine);
  const [triggers, setTriggers] = useState<RoutineTrigger[] | null>(initialTriggers);
  const [draft, setDraft] = useState<DraftState>(buildDraft(initialRoutine, initialTriggers));
  const [saving, setSaving] = useState(false);
  const update = useCallback(
    (patch: Partial<DraftState>) => setDraft((d) => ({ ...d, ...patch })),
    [],
  );

  const cronTrigger = triggers?.find((t) => t.kind === "cron");
  const lastFired = cronTrigger?.lastFiredAt ?? null;

  const handleSave = useCallback(async () => {
    if (triggers === null) {
      toast.error(t("office:routineScheduleFateUnknown"));
      return;
    }
    setSaving(true);
    try {
      await updateRoutine(routine.id, buildUpdatePatch(draft));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToSaveRoutine"));
      setSaving(false);
      return;
    }

    try {
      const outcome = await reconcileCronTrigger(routine.id, draft, triggers);
      if (outcome.kind !== "unchanged") setTriggers(outcome.triggers);
      const result = describeCronOutcome(outcome, t);
      if (result.toastKind === "success") {
        toast.success(result.message);
      } else {
        toast.error(result.message);
      }
      if (result.refresh) router.refresh();
    } catch (err) {
      // reconcileCronTrigger is designed to always resolve (every internal
      // call is its own try/catch) rather than throw, but nothing enforces
      // that contract. Catching here — not just the `finally` below — keeps
      // a future violation from becoming an unhandled rejection out of an
      // unawaited click handler on top of a stuck Save button.
      toast.error(err instanceof Error ? err.message : t("office:failedToSaveRoutine"));
    } finally {
      setSaving(false);
    }
  }, [routine.id, draft, triggers, router, t]);

  const handleRunNow = useCallback(async () => {
    try {
      await runRoutine(routine.id);
      toast.success(t("office:routineFired"));
    } catch (err) {
      toast.error(routineNotFiringMessage(err, t, "office:failedToRunRoutine"));
    }
  }, [routine.id]);

  useOfficeTopbar({
    // The draft is the live name; `routine` is frozen at mount, so a saved
    // rename would otherwise keep the old title until a reload.
    title: draft.name,
    parents: [{ label: t("office:routines"), href: "/office/routines" }],
    actions: (
      <>
        <Button size="sm" variant="outline" onClick={handleRunNow} className="cursor-pointer">
          <IconPlayerPlay className="h-4 w-4 mr-1" /> {t("office:runNow")}
        </Button>
        <Button size="sm" onClick={handleSave} disabled={saving} className="cursor-pointer">
          <IconDeviceFloppy className="h-4 w-4 mr-1" />{" "}
          {saving ? t("office:savingEllipsis") : t("common:save")}
        </Button>
      </>
    ),
  });

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <DetailGeneralCard draft={draft} update={update} agents={agents} />
      <DetailTriggerCard draft={draft} update={update} />
      <DetailReadOnlyCard
        routine={routine}
        lastFiredAt={lastFired}
        nextRunAt={
          isRoutineFiring(draft.status ?? routine.status) ? (cronTrigger?.nextRunAt ?? null) : null
        }
      />
    </div>
  );
}

function DetailGeneralCard({
  draft,
  update,
  agents,
}: {
  draft: DraftState;
  update: (patch: Partial<DraftState>) => void;
  agents: Array<{ id: string; name: string }>;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("office:general")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <BasicGeneralFields draft={draft} update={update} />
        <StatusAndAssigneeFields draft={draft} update={update} agents={agents} />
        <PolicyFields draft={draft} update={update} />
        {draft.catchUpPolicy === "summarize_missed" && (
          <Field label={t("office:catchUpMax")}>
            <Input
              type="number"
              min={1}
              value={draft.catchUpMax}
              onChange={(e) => update({ catchUpMax: e.target.value })}
              onBlur={(e) => update({ catchUpMax: String(coerceCatchUpMax(e.target.value)) })}
            />
          </Field>
        )}
      </CardContent>
    </Card>
  );
}

function BasicGeneralFields({
  draft,
  update,
}: {
  draft: DraftState;
  update: (patch: Partial<DraftState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Field label={t("office:name")}>
        <Input value={draft.name} onChange={(e) => update({ name: e.target.value })} />
      </Field>
      <Field label={t("office:description")}>
        <Textarea
          rows={2}
          value={draft.description}
          onChange={(e) => update({ description: e.target.value })}
        />
      </Field>
    </>
  );
}

function StatusAndAssigneeFields({
  draft,
  update,
  agents,
}: {
  draft: DraftState;
  update: (patch: Partial<DraftState>) => void;
  agents: Array<{ id: string; name: string }>;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-4">
      <Field label={t("common:status")}>
        <Select
          value={draft.status ?? ""}
          onValueChange={(v) => update({ status: v as DraftState["status"] })}
        >
          <SelectTrigger className="cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active" className="cursor-pointer">
              {t("office:routineStatusActive")}
            </SelectItem>
            <SelectItem value="paused" className="cursor-pointer">
              {t("office:routineStatusPaused")}
            </SelectItem>
            <SelectItem value="archived" className="cursor-pointer">
              {t("office:routineStatusArchived")}
            </SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field label={t("office:assignee")}>
        <Select
          value={draft.assigneeAgentProfileId}
          onValueChange={(v) => update({ assigneeAgentProfileId: v })}
        >
          <SelectTrigger className="cursor-pointer">
            <SelectValue placeholder={t("office:unassigned")} />
          </SelectTrigger>
          <SelectContent>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
    </div>
  );
}

function PolicyFields({
  draft,
  update,
}: {
  draft: DraftState;
  update: (patch: Partial<DraftState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-4">
      <Field label={t("office:concurrencyPolicy")}>
        <Select
          value={draft.concurrencyPolicy}
          onValueChange={(v) => update({ concurrencyPolicy: v })}
        >
          <SelectTrigger className="cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="skip_if_active" className="cursor-pointer">
              {t("office:skipIfActive")}
            </SelectItem>
            <SelectItem value="coalesce_if_active" className="cursor-pointer">
              {t("office:coalesceIfActive")}
            </SelectItem>
            <SelectItem value="always_create" className="cursor-pointer">
              {t("office:alwaysCreate")}
            </SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field label={t("office:catchUpPolicy")}>
        <Select value={draft.catchUpPolicy} onValueChange={(v) => update({ catchUpPolicy: v })}>
          <SelectTrigger className="cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="summarize_missed" className="cursor-pointer">
              {t("office:summarizeMissed")}
            </SelectItem>
            <SelectItem value="skip_missed" className="cursor-pointer">
              {t("office:skipMissed")}
            </SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </div>
  );
}

function DetailTriggerCard({
  draft,
  update,
}: {
  draft: DraftState;
  update: (patch: Partial<DraftState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("office:trigger")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <Field label={t("office:kind")}>
            <Select
              value={draft.triggerKind}
              onValueChange={(v) => update({ triggerKind: v as DraftState["triggerKind"] })}
            >
              <SelectTrigger className="cursor-pointer">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="cron" className="cursor-pointer">
                  {t("office:cron")}
                </SelectItem>
                <SelectItem value="webhook" className="cursor-pointer">
                  {t("office:webhook")}
                </SelectItem>
              </SelectContent>
            </Select>
          </Field>
          {draft.triggerKind === "cron" && (
            <Field label={t("office:cronExpression")}>
              <Input
                value={draft.cronExpression}
                onChange={(e) => update({ cronExpression: e.target.value })}
                placeholder="*/5 * * * *"
              />
            </Field>
          )}
        </div>
        {draft.triggerKind === "cron" && (
          <Field label={t("office:timezone")}>
            <Input
              value={draft.timezone}
              onChange={(e) => update({ timezone: e.target.value })}
              placeholder="UTC"
            />
          </Field>
        )}
      </CardContent>
    </Card>
  );
}

function DetailReadOnlyCard({
  routine,
  lastFiredAt,
  nextRunAt,
}: {
  routine: Routine;
  lastFiredAt: string | null;
  nextRunAt: string | null;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("office:schedule")}</CardTitle>
      </CardHeader>
      <CardContent className="text-sm text-muted-foreground space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <ScheduleStateBadge routine={routine} />
          <UnarmedScheduleHint routine={routine} />
        </div>
        {/* `{{when}}` carries a formatted timestamp, not a translated label. */}
        <div>
          {t("office:lastFired", { when: lastFiredAt ? timeAgo(lastFiredAt) : t("office:never") })}
        </div>
        <div>
          {t("office:nextFire", {
            when: nextRunAt ? new Date(nextRunAt).toLocaleString() : "-",
          })}
        </div>
      </CardContent>
    </Card>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  );
}
