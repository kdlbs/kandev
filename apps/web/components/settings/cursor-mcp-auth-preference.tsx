"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { SettingsFieldDescription } from "@/components/settings/settings-typography";

export function CursorMCPAuthPreference({
  enabled,
  savedEnabled,
  onChange,
}: {
  enabled: boolean;
  savedEnabled?: boolean;
  onChange: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  const instanceId = useId();
  const labelId = `${instanceId}-cursor-mcp-auth-label`;
  const descriptionId = `${instanceId}-cursor-mcp-auth-description`;
  const dirty = savedEnabled !== undefined && enabled !== savedEnabled;

  return (
    <div
      className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3 sm:min-h-0"
      data-settings-dirty={dirty}
      data-settings-dirty-level="container"
      data-testid="cursor-mcp-auth-preference"
      onClick={() => onChange(!enabled)}
    >
      <Checkbox
        checked={enabled}
        onCheckedChange={(value) => onChange(value === true)}
        onClick={(event) => event.stopPropagation()}
        aria-labelledby={labelId}
        aria-describedby={descriptionId}
        className="mt-0.5 flex-none"
      />
      <span className="min-w-0 flex-1">
        <span id={labelId} className="block text-sm font-medium leading-5">
          {t("agents:cursorMcpAuthLabel")}
        </span>
        <SettingsFieldDescription id={descriptionId}>
          {t("agents:cursorMcpAuthDescription")}
        </SettingsFieldDescription>
        {!enabled && (
          <SettingsFieldDescription className="mt-1">
            {t("agents:cursorMcpAuthDisabledNote")}
          </SettingsFieldDescription>
        )}
      </span>
    </div>
  );
}
