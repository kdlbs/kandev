"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { IconPencil } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { getDefaultCeiling, setDefaultCeiling } from "@/lib/api/domains/office-api";
import { formatDollars } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type Props = {
  workspaceId: string;
};

type EditorProps = {
  draftDollars: string;
  saving: boolean;
  onDraftChange: (value: string) => void;
  onSave: () => void;
  onCancel: () => void;
};

function DefaultCeilingEditor({
  draftDollars,
  saving,
  onDraftChange,
  onSave,
  onCancel,
}: EditorProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
      <Input
        className="h-8 w-full text-sm [@media(pointer:coarse)]:min-h-11 sm:w-32"
        type="number"
        min="0.01"
        step="0.01"
        value={draftDollars}
        onChange={(e) => onDraftChange(e.target.value)}
        aria-label={t("office:defaultCeiling")}
        autoFocus
      />
      <Button
        size="sm"
        className="w-full cursor-pointer [@media(pointer:coarse)]:min-h-11 sm:w-auto"
        disabled={saving}
        onClick={onSave}
      >
        {t("office:saveDefaultCeiling")}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="w-full cursor-pointer [@media(pointer:coarse)]:min-h-11 sm:w-auto"
        disabled={saving}
        onClick={onCancel}
      >
        {t("common:cancel")}
      </Button>
    </div>
  );
}

// DefaultCeilingCard makes the built-in default spend ceiling
// (AC-OFFICE-BUDGET-003.5) visible and editable without inspecting the
// database. It is visually and structurally distinct from a
// BudgetPolicyCard: no scope/action/period controls, since the default is
// always workspace-scoped, always daily, and always blocking
// (AC-OFFICE-BUDGET-003.2/.3) — only its limit is operator-tunable.
export function DefaultCeilingCard({ workspaceId }: Props) {
  const { t } = useTranslation();
  const [limitSubcents, setLimitSubcents] = useState<number | null>(null);
  const [editing, setEditing] = useState(false);
  const [draftDollars, setDraftDollars] = useState("");
  const [saving, setSaving] = useState(false);
  const activeWorkspaceId = useRef(workspaceId);

  useEffect(() => {
    activeWorkspaceId.current = workspaceId;
    setLimitSubcents(null);
    setEditing(false);
    setDraftDollars("");
    setSaving(false);
    getDefaultCeiling(workspaceId)
      .then((res) => {
        if (activeWorkspaceId.current !== workspaceId) return;
        setLimitSubcents(res.limit_subcents);
      })
      .catch((err) => {
        if (activeWorkspaceId.current !== workspaceId) return;
        toast.error(err instanceof Error ? err.message : t("office:failedToLoadDefaultCeiling"));
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- toast/t are stable; re-fetch only on workspace change
  }, [workspaceId]);

  const startEditing = () => {
    setDraftDollars(limitSubcents == null ? "" : (limitSubcents / 10000).toFixed(2));
    setEditing(true);
  };

  const handleSave = async () => {
    const draftValue = Number(draftDollars);
    if (!Number.isFinite(draftValue) || draftValue < 0.01) {
      toast.error(t("office:defaultCeilingMustBePositive"));
      return;
    }
    const nextSubcents = Math.round(draftValue * 10000);
    const targetWorkspaceId = workspaceId;
    setSaving(true);
    try {
      const res = await setDefaultCeiling(targetWorkspaceId, nextSubcents);
      if (activeWorkspaceId.current !== targetWorkspaceId) return;
      setLimitSubcents(res.limit_subcents);
      setEditing(false);
      toast.success(t("office:defaultCeilingUpdated"));
    } catch (err) {
      if (activeWorkspaceId.current !== targetWorkspaceId) return;
      toast.error(err instanceof Error ? err.message : t("office:failedToUpdateDefaultCeiling"));
    } finally {
      if (activeWorkspaceId.current === targetWorkspaceId) setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm">{t("office:defaultCeiling")}</CardTitle>
        {!editing && (
          <Button
            variant="ghost"
            size="icon-sm"
            className="cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
            onClick={startEditing}
            aria-label={t("office:editDefaultCeiling")}
          >
            <IconPencil className="h-4 w-4" />
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="text-xs text-muted-foreground">{t("office:defaultCeilingDescription")}</p>
        {editing ? (
          <DefaultCeilingEditor
            draftDollars={draftDollars}
            saving={saving}
            onDraftChange={setDraftDollars}
            onSave={handleSave}
            onCancel={() => setEditing(false)}
          />
        ) : (
          <div className="text-lg font-semibold">
            {limitSubcents == null ? t("common:loading") : formatDollars(limitSubcents)}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
