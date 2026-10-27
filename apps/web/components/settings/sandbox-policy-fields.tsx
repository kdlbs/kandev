"use client";

import { settingsActionClassName, settingsControlClassName } from "./settings-control";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Label } from "@kandev/ui/label";
import { Checkbox } from "@kandev/ui/checkbox";
import { Button } from "@kandev/ui/button";
import { generateUUID } from "@/lib/uuid";
import type { SandboxAccessMode, SandboxPolicyDraft } from "@/lib/sandbox-policy-draft";

type Props = { draft: SandboxPolicyDraft; update: (patch: Partial<SandboxPolicyDraft>) => void };

export function SandboxPolicyFields({ draft, update }: Props) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <Label htmlFor={`${id}-name`}>{t("agents:sandboxPolicyName")}</Label>
        <Input
          id={`${id}-name`}
          value={draft.name}
          maxLength={128}
          onChange={(event) => update({ name: event.target.value })}
        />
      </div>
      <AccessField
        label={t("agents:sandboxWorkDirectory")}
        value={draft.workDir}
        onChange={(workDir) => update({ workDir })}
      />
      <AccessField
        label={t("agents:sandboxAgentState")}
        value={draft.agentState}
        onChange={(agentState) => update({ agentState })}
      />
      <LinesField
        label={t("agents:sandboxReadPaths")}
        value={draft.readPaths}
        onChange={(readPaths) => update({ readPaths })}
      />
      <LinesField
        label={t("agents:sandboxWritePaths")}
        value={draft.writePaths}
        onChange={(writePaths) => update({ writePaths })}
      />
      <label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm">
        <Checkbox
          checked={draft.mcp}
          onCheckedChange={(checked) => update({ mcp: checked === true })}
        />
        {t("agents:sandboxAllowMCP")}
      </label>
      <SandboxNetworkFields draft={draft} update={update} />
      <LinesField
        label={t("agents:sandboxEnvironmentNames")}
        value={draft.environmentKeys}
        onChange={(environmentKeys) => update({ environmentKeys })}
      />
    </div>
  );
}

function AccessField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: SandboxAccessMode;
  onChange: (value: SandboxAccessMode) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <select
        id={id}
        className={settingsControlClassName(
          "w-full rounded-md border border-input bg-background px-3",
        )}
        value={value}
        onChange={(event) => onChange(event.target.value as SandboxAccessMode)}
      >
        <option value="none">{t("agents:sandboxAccessNone")}</option>
        <option value="read">{t("agents:sandboxAccessRead")}</option>
        <option value="write">{t("agents:sandboxAccessWrite")}</option>
      </select>
    </div>
  );
}

function LinesField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const id = useId();
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Textarea id={id} value={value} rows={2} onChange={(event) => onChange(event.target.value)} />
    </div>
  );
}

function SandboxNetworkFields({ draft, update }: Props) {
  const { t } = useTranslation();
  return (
    <fieldset className="space-y-3">
      <legend className="text-sm font-medium">{t("agents:sandboxNetwork")}</legend>
      {draft.network.map((row) => (
        <NetworkRow
          key={row.id}
          row={row}
          update={(patch) =>
            update({
              network: draft.network.map((item) =>
                item.id === row.id ? { ...item, ...patch } : item,
              ),
            })
          }
          remove={() => update({ network: draft.network.filter((item) => item.id !== row.id) })}
        />
      ))}
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName()}
        disabled={draft.network.length >= 256}
        onClick={() =>
          update({
            network: [
              ...draft.network,
              { id: generateUUID(), host: "", port: "", protocol: "tcp" },
            ],
          })
        }
      >
        {t("agents:sandboxAddDestination")}
      </Button>
    </fieldset>
  );
}

function NetworkRow({
  row,
  update,
  remove,
}: {
  row: SandboxPolicyDraft["network"][number];
  update: (patch: Partial<SandboxPolicyDraft["network"][number]>) => void;
  remove: () => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <div className="space-y-3 rounded-md border p-3">
      <div className="space-y-2">
        <Label htmlFor={`${id}-host`}>{t("agents:sandboxDestinationHost")}</Label>
        <Input
          id={`${id}-host`}
          value={row.host}
          onChange={(event) => update({ host: event.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label htmlFor={`${id}-port`}>{t("agents:sandboxDestinationPort")}</Label>
          <Input
            id={`${id}-port`}
            inputMode="numeric"
            value={row.port}
            onChange={(event) => update({ port: event.target.value })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${id}-protocol`}>{t("agents:sandboxDestinationProtocol")}</Label>
          <select
            id={`${id}-protocol`}
            className={settingsControlClassName(
              "w-full rounded-md border border-input bg-background px-3",
            )}
            value={row.protocol}
            onChange={(event) => update({ protocol: event.target.value as typeof row.protocol })}
          >
            {(["tcp", "http", "https"] as const).map((value) => (
              <option key={value} value={value}>
                {value.toUpperCase()}
              </option>
            ))}
          </select>
        </div>
      </div>
      <Button type="button" variant="ghost" className={settingsActionClassName()} onClick={remove}>
        {t("agents:remove")}
      </Button>
    </div>
  );
}
