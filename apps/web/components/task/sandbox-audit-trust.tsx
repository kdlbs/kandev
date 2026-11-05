"use client";

import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { useAppStore } from "@/components/state-provider";
import type { AuditModel } from "./sandbox-audit-view";
import { sandboxReportActionClass as auditActionClass } from "./sandbox-report-header";

export function SandboxAuditTrustSettings({ model }: { model: AuditModel }) {
  const { t } = useTranslation();
  const trust = model.trust;
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item) => item.id === trust?.workspace_id)?.name,
  );
  if (!trust) return null;
  let status = "agents:auditTrustConfigured";
  if (trust.version === 0) status = "agents:auditTrustUnconfigured";
  else if (trust.revoked) status = "agents:auditTrustRevoked";
  return (
    <section className="min-w-0 space-y-3" data-testid="sandbox-audit-trust">
      <h3 className="font-medium">{t("agents:auditTrustTitle")}</h3>
      <p className="text-muted-foreground">
        {t("agents:auditTrustScope", {
          scope: workspace || trust.workspace_id || t("agents:auditOrganization"),
          provider: trust.provider_key,
        })}
      </p>
      <p>{t(status, { version: trust.version })}</p>
      {trust.version > 0 && (
        <p className="break-words text-muted-foreground">
          {t("agents:auditTrustProvenance", {
            operator: trust.operator_id,
            time: trust.created_at,
          })}
        </p>
      )}
      {trust.fingerprint && (
        <p className="break-all">
          <span className="text-muted-foreground">{t("agents:auditFingerprint")}: </span>
          <code>{trust.fingerprint}</code>
        </p>
      )}
      {model.operator && <AuditTrustForm model={model} trust={trust} />}
    </section>
  );
}

function AuditTrustForm({
  model,
  trust,
}: {
  model: AuditModel;
  trust: NonNullable<AuditModel["trust"]>;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [draft, setDraft] = useState<{ version: number; pem: string } | null>(null);
  const pem = draft?.pem ?? trust.public_key_pem;
  const version = draft?.version ?? trust.version;
  const stale = version !== trust.version;
  const canSave =
    model.canManageTrust &&
    !stale &&
    pem.startsWith("-----BEGIN PUBLIC KEY-----") &&
    !pem.includes("PRIVATE KEY") &&
    pem !== trust.public_key_pem;
  async function save() {
    if (await model.saveTrust({ expected_version: version, public_key_pem: pem })) setDraft(null);
  }
  async function revoke() {
    if (await model.saveTrust({ expected_version: version, revoke: true })) setDraft(null);
  }
  return (
    <>
      <div className="space-y-2">
        <Label htmlFor={id}>{t("agents:auditPublicKey")}</Label>
        <Textarea
          id={id}
          value={pem}
          onChange={(event) => setDraft({ version, pem: event.target.value })}
          maxLength={16_384}
          disabled={!model.canManageTrust}
          aria-describedby={`${id}-help`}
          className="min-h-28 font-mono text-xs break-all"
          spellCheck={false}
          autoComplete="off"
        />
        <p id={`${id}-help`} className="text-muted-foreground">
          {t("agents:auditPublicKeyHelp")}
        </p>
      </div>
      {stale && <p role="alert">{t("agents:auditTrustStale")}</p>}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          className={auditActionClass}
          disabled={!canSave}
          onClick={() => void save()}
        >
          {t("agents:auditTrustSave")}
        </Button>
        <Button
          type="button"
          variant="outline"
          className={auditActionClass}
          disabled={!model.canManageTrust || trust.revoked || stale}
          onClick={() => void revoke()}
        >
          {t("agents:auditTrustRevoke")}
        </Button>
        {draft && (
          <Button
            type="button"
            variant="ghost"
            className={auditActionClass}
            disabled={model.isBusy}
            onClick={() => setDraft(null)}
          >
            {t("agents:auditTrustReset")}
          </Button>
        )}
      </div>
    </>
  );
}
