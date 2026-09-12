"use client";

import { useTranslation } from "react-i18next";
import type { CanvasPermissionReview } from "@/lib/api/domains/canvas-api";
import {
  buildCanvasPermissionGroups,
  type CanvasPermissionGroup,
} from "@/lib/canvas-permission-copy";

export function hasUnsupportedPermissions(groups: CanvasPermissionGroup[]): boolean {
  return groups.some((group) => group.rows.some((row) => row.isUnsupported));
}

export function CanvasPermissionSummary({
  permissions,
  missingPermissions,
}: {
  permissions: CanvasPermissionReview | undefined;
  missingPermissions?: string[];
}) {
  const { t } = useTranslation();
  const groups = buildCanvasPermissionGroups(permissions, missingPermissions, t);
  if (groups.length === 0) return null;

  return (
    <div
      className="space-y-3 rounded-md border bg-muted/20 p-3"
      data-testid="canvas-permission-summary"
    >
      <p className="font-medium">{t("canvases:permissionDeclaration")}</p>
      {groups.map((group) => (
        <section key={group.id}>
          <h3 className="font-medium">{group.label}</h3>
          <ul className="mt-1 space-y-2">
            {group.rows.map((row) => (
              <li key={row.id} className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
                <span className={row.isUnsupported ? "text-destructive" : undefined}>
                  {row.label}
                </span>
                {row.detail && (
                  <code className="break-all text-xs text-muted-foreground">{row.detail}</code>
                )}
                {row.isNew && (
                  <span className="rounded bg-primary/10 px-1.5 py-0.5 text-xs font-medium text-primary">
                    {t("canvases:newPermission")}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
