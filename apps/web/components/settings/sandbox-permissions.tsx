"use client";

import { useTranslation } from "react-i18next";
import type { SandboxGrants } from "@/lib/types/sandbox";

export function SandboxPermissions({ grants }: { grants: SandboxGrants }) {
  const { t } = useTranslation();
  const read = grants.filesystem;
  const write = grants.filesystem.filter((item) => item.writable);
  const location = (item: SandboxGrants["filesystem"][number]) =>
    item.path || `{{kandev.${item.reference}}}`;
  return (
    <dl className="space-y-3 text-xs" data-testid="sandbox-permissions">
      <PermissionRow label={t("agents:sandboxReadable")} values={read.map(location)} />
      <PermissionRow label={t("agents:sandboxWritable")} values={write.map(location)} />
      <PermissionRow
        label={t("agents:sandboxNetwork")}
        values={grants.network.map((item) => `${item.protocol}://${item.host}:${item.port}`)}
      />
      <div>
        <dt className="font-medium">{t("agents:sandboxMCP")}</dt>
        <dd className="mt-1 text-muted-foreground">
          {t(grants.kandev_mcp ? "agents:sandboxMCPAllowed" : "agents:sandboxMCPDenied")}
        </dd>
      </div>
      <PermissionRow
        label={t("agents:sandboxEnvironment")}
        values={grants.environment_keys ?? []}
      />
      <div>
        <dt className="font-medium">{t("agents:sandboxRestrictions")}</dt>
        <dd className="mt-1 text-muted-foreground">{t("agents:sandboxRestrictionHelp")}</dd>
      </div>
    </dl>
  );
}

function PermissionRow({ label, values }: { label: string; values: string[] }) {
  const { t } = useTranslation();
  return (
    <div>
      <dt className="font-medium">{label}</dt>
      <dd className="mt-1 space-y-1 break-all text-muted-foreground">
        {values.length
          ? values.map((value) => (
              <div key={value}>
                <code>{value}</code>
              </div>
            ))
          : t("agents:sandboxNoGrants")}
      </dd>
    </div>
  );
}
