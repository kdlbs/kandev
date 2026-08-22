"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { useToast } from "@/components/toast-provider";
import {
  getDefaultWorkspaceVisibility,
  setDefaultWorkspaceVisibility,
} from "@/lib/api/domains/team-access-api";
import type { WorkspaceVisibility } from "@/lib/types/team-access";

/**
 * The install-wide default for newly created workspaces.
 *
 * This is the control that makes a shared board a one-time decision rather
 * than a per-workspace chore. It deliberately never touches existing
 * workspaces: flipping the default must not retroactively publish work that
 * was private a moment ago.
 */
export function DefaultVisibilityCard() {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [visibility, setVisibility] = useState<WorkspaceVisibility | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    getDefaultWorkspaceVisibility()
      .then((response) => setVisibility(response.visibility))
      .catch(() => setVisibility(null));
  }, []);

  const onChange = useCallback(
    (next: string) => {
      const value = next as WorkspaceVisibility;
      const previous = visibility;
      setVisibility(value);
      setBusy(true);
      setDefaultWorkspaceVisibility(value)
        .then(() =>
          toast({
            title:
              value === "org"
                ? t("workspaces:defaultVisibility.savedOrg")
                : t("workspaces:defaultVisibility.savedPrivate"),
          }),
        )
        .catch((error: unknown) => {
          setVisibility(previous);
          toast({
            title: error instanceof Error ? error.message : t("workspaces:teamAccess.genericError"),
            variant: "error",
          });
        })
        .finally(() => setBusy(false));
    },
    [visibility, toast, t],
  );

  if (visibility === null) return null;

  return (
    <SettingsCard>
      <CardHeader>
        <CardTitle>{t("workspaces:defaultVisibility.title")}</CardTitle>
        <CardDescription>{t("workspaces:defaultVisibility.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Label htmlFor="default-workspace-visibility">
          {t("workspaces:defaultVisibility.label")}
        </Label>
        <Select value={visibility} onValueChange={onChange} disabled={busy}>
          <SelectTrigger id="default-workspace-visibility" className="max-w-sm cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="private">{t("workspaces:teamAccess.visibilityPrivate")}</SelectItem>
            <SelectItem value="org">{t("workspaces:teamAccess.visibilityOrg")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-muted-foreground text-sm">
          {t("workspaces:defaultVisibility.existingUnaffected")}
        </p>
      </CardContent>
    </SettingsCard>
  );
}
