"use client";

import { useEffect, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useTranslation } from "react-i18next";

type StepWorkspaceProps = {
  workspaceName: string;
  taskPrefix: string;
  onChange: (patch: { workspaceName?: string; taskPrefix?: string }) => void;
};

export function derivePrefix(name: string): string {
  const cleaned = name.replace(/[^a-zA-Z0-9]/g, "").toUpperCase();
  return cleaned.slice(0, 3) || "KAN";
}

export function StepWorkspace({ workspaceName, taskPrefix, onChange }: StepWorkspaceProps) {
  const { t } = useTranslation();
  const [prefixDirty, setPrefixDirty] = useState(false);

  useEffect(() => {
    if (prefixDirty) return;
    const derived = derivePrefix(workspaceName);
    if (derived !== taskPrefix) onChange({ taskPrefix: derived });
    // onChange/taskPrefix intentionally omitted: we only resync when workspace name or
    // dirty flag changes, otherwise this would fight with the parent's state updates.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceName, prefixDirty]);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{t("office:setUpYourOfficeWorkspace")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("office:officeTurnsYourBacklogIntoAutonomous")}
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="workspace-name">{t("office:workspaceName")}</Label>
          <Input
            id="workspace-name"
            value={workspaceName}
            onChange={(e) => onChange({ workspaceName: e.target.value })}
            placeholder={t("office:defaultWorkspace")}
            className="mt-1"
            autoFocus
          />
          <p className="text-xs text-muted-foreground mt-1">
            {t("office:aNameForYourWorkspaceYou")}
          </p>
        </div>
        <div>
          <Label htmlFor="task-prefix">{t("office:taskPrefix")}</Label>
          <Input
            id="task-prefix"
            value={taskPrefix}
            onChange={(e) => {
              setPrefixDirty(true);
              onChange({ taskPrefix: e.target.value.toUpperCase() });
            }}
            placeholder="KAN"
            className="mt-1 max-w-32"
            maxLength={6}
          />
          {/* `"KAN"` is the exact prefix `submitOnboarding` persists when the
              field is left empty, so the preview keeps it literal. */}
          <p className="text-xs text-muted-foreground mt-1">
            {t("office:tasksWillBeNumbered", { prefix: taskPrefix || "KAN" })}
          </p>
        </div>
      </div>
    </div>
  );
}
