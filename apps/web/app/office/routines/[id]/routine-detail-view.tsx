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
import {
  updateRoutine,
  runRoutine,
  createRoutineTrigger,
  deleteRoutineTrigger,
  listRoutineTriggers,
} from "@/lib/api/domains/office-api";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import { timeAgo } from "@/lib/utils/time";
import { useOfficeTopbar } from "../../components/office-topbar-context";
import { isRoutineFiring } from "../../lib/routine-status";
import { routineNotFiringMessage } from "../../lib/routine-not-firing";
import { selectPrimaryCronTrigger } from "../../lib/routine-trigger-selection";
import { useTranslation } from "react-i18next";

// The timezone control is free text with "UTC" as its placeholder, not a
// value, so a cleared box drafts "". This is the one default both the draft
// seed (AC-003.7) and trigger sync's comparison (AC-004.12) resolve to.
const DEFAULT_TIMEZONE = "UTC";

// Lift the form state out of the component so the file stays under the
// 100-line per-function ceiling and the helpers can render typed slices
// of the draft without re-deriving every field on each call.
type DraftState = {
  name: string;
  description: string;
  status: "active" | "paused" | "archived";
  assigneeAgentProfileId: string;
  concurrencyPolicy: string;
  catchUpPolicy: string;
  catchUpMax: number;
  triggerKind: "cron" | "webhook";
  cronExpression: string;
  timezone: string;
};

function pickTriggerKind(triggers: RoutineTrigger[]): "cron" | "webhook" {
  const cron = triggers.find((t) => t.kind === "cron");
  if (cron) return "cron";
  const webhook = triggers.find((t) => t.kind === "webhook");
  if (webhook) return "webhook";
  return "cron";
}

// Seeds the editable cron/timezone fields from the same trigger AC-003.5
// resolves (the REQ-003 primary, else its fallback), not by array position:
// these values are one operand of AC-004.4's comparison against trigger
// sync's target, which the selector also resolves.
function buildDraft(routine: Routine, triggers: RoutineTrigger[]): DraftState {
  const { trigger: cron } = selectPrimaryCronTrigger(triggers);
  const triggerKind = pickTriggerKind(triggers);
  return {
    name: routine.name,
    description: routine.description ?? "",
    status: (routine.status as DraftState["status"]) ?? "active",
    assigneeAgentProfileId: routine.assigneeAgentProfileId ?? "",
    concurrencyPolicy: routine.concurrencyPolicy ?? "coalesce_if_active",
    catchUpPolicy: routine.catchUpPolicy ?? "summarize_missed",
    catchUpMax: routine.catchUpMax ?? 25,
    triggerKind,
    cronExpression: cron?.cronExpression ?? "",
    timezone: cron?.timezone ?? DEFAULT_TIMEZONE,
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
  const [triggers, setTriggers] = useState(initialTriggers);
  const [draft, setDraft] = useState<DraftState>(buildDraft(initialRoutine, initialTriggers));
  const [saving, setSaving] = useState(false);
  const update = useCallback(
    (patch: Partial<DraftState>) => setDraft((d) => ({ ...d, ...patch })),
    [],
  );

  const { trigger: cronSelection, isPrimary } = selectPrimaryCronTrigger(triggers);
  const lastFired = cronSelection?.lastFiredAt ?? null;

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await updateRoutine(routine.id, {
        name: draft.name,
        description: draft.description,
        status: draft.status,
        assigneeAgentProfileId: draft.assigneeAgentProfileId,
        concurrencyPolicy: draft.concurrencyPolicy,
        catchUpPolicy: draft.catchUpPolicy,
        catchUpMax: draft.catchUpMax,
      } as Record<string, unknown>);
      const outcome = await syncCronTrigger(routine.id, draft, triggers);
      setTriggers(outcome.triggers);
      if (outcome.errorKey) {
        toast.error(t(outcome.errorKey));
      } else {
        toast.success(t("office:routineSaved"));
        router.refresh();
      }
    } catch (err) {
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
        lastFiredAt={lastFired}
        nextRunAt={
          isRoutineFiring(draft.status) && isPrimary ? (cronSelection?.nextRunAt ?? null) : null
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
              onChange={(e) => update({ catchUpMax: Number(e.target.value) || 25 })}
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
          value={draft.status}
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
  lastFiredAt,
  nextRunAt,
}: {
  lastFiredAt: string | null;
  nextRunAt: string | null;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("office:schedule")}</CardTitle>
      </CardHeader>
      <CardContent className="text-sm text-muted-foreground space-y-1">
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

type SyncOutcome = {
  triggers: RoutineTrigger[];
  errorKey?: string;
};

// syncCronTrigger reconciles the routine's cron trigger with the draft's
// expression / timezone / kind against the REQ-003 primary selection (the
// same trigger buildDraft seeded from), so this call's target always agrees
// with what the page displayed. Draft.kind === "webhook" or an empty
// expression leaves triggers alone. The trigger model has no PATCH endpoint,
// so a changed expression is a delete-then-create; a delete that succeeds
// but whose create then fails must not silently keep the old schedule, so
// callers get back which cron trigger (if any) survived plus an optional
// localized error key instead of a thrown generic failure.
async function syncCronTrigger(
  routineId: string,
  draft: DraftState,
  triggers: RoutineTrigger[],
): Promise<SyncOutcome> {
  if (draft.triggerKind !== "cron" || !draft.cronExpression.trim()) {
    return { triggers };
  }
  const resolvedTimezone = draft.timezone === "" ? DEFAULT_TIMEZONE : draft.timezone;
  const { trigger: target } = selectPrimaryCronTrigger(triggers);

  if (!target) {
    const created = await createRoutineTrigger(routineId, {
      kind: "cron",
      cronExpression: draft.cronExpression,
      timezone: resolvedTimezone,
    });
    if (created.trigger) return { triggers: [...triggers, created.trigger] };
    return refetchTriggersAfterUnusableCreate(routineId, triggers);
  }

  if (target.cronExpression === draft.cronExpression && target.timezone === resolvedTimezone) {
    return { triggers };
  }

  await deleteRoutineTrigger(target.id);
  const remaining = triggers.filter((t) => t.id !== target.id);
  let created: { trigger: RoutineTrigger | null };
  try {
    created = await createRoutineTrigger(routineId, {
      kind: "cron",
      cronExpression: draft.cronExpression,
      timezone: resolvedTimezone,
    });
  } catch {
    const hasOtherCron = remaining.some((t) => t.kind === "cron");
    return {
      triggers: remaining,
      errorKey: hasOtherCron
        ? "office:triggerSyncFailedKeptPrevious"
        : "office:triggerSyncFailedNoSchedule",
    };
  }
  if (created.trigger) return { triggers: [...remaining, created.trigger] };
  return refetchTriggersAfterUnusableCreate(routineId, remaining);
}

// A create call that returns a null trigger (the API layer's signal for an
// unusable/empty response body) leaves the true post-delete state unknown,
// so re-read it from the server rather than guessing the delete "won".
async function refetchTriggersAfterUnusableCreate(
  routineId: string,
  fallbackTriggers: RoutineTrigger[],
): Promise<SyncOutcome> {
  try {
    const { triggers: refetched } = await listRoutineTriggers(routineId);
    if (refetched.some((t) => t.kind === "cron")) {
      return { triggers: refetched };
    }
    return { triggers: fallbackTriggers, errorKey: "office:triggerSyncReadBackFailed" };
  } catch {
    return { triggers: fallbackTriggers, errorKey: "office:triggerSyncReadBackFailed" };
  }
}
