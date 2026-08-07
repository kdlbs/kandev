"use client";

import { useEffect, useState } from "react";
import { IconEye, IconEyeOff } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useTranslation } from "react-i18next";

export function GitHubPATForm({
  workspaceId,
  value,
  onChange,
  disabled,
}: {
  workspaceId: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    setVisible(false);
  }, [workspaceId]);

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="github-workspace-token">{t("github:personalAccessToken")}</Label>
        <p className="text-xs text-muted-foreground">{t("github:kandevStoresThisTokenForThis")}</p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <div className="relative min-w-0 flex-1">
          <Input
            id="github-workspace-token"
            type={visible ? "text" : "password"}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            placeholder="ghp_xxxxxxxxxxxx"
            autoComplete="off"
            disabled={disabled}
            className="h-11 pr-11 font-mono"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="absolute right-0 top-0 h-11 w-11 cursor-pointer"
            onClick={() => setVisible((current) => !current)}
            disabled={disabled}
            aria-label={visible ? t("github:hideToken") : t("github:showToken")}
          >
            {visible ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
          </Button>
        </div>
      </div>
    </div>
  );
}
