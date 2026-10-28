"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { ManagedSSHConnection } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName, settingsControlClassName } from "../settings-control";
import { SettingsInfo } from "../settings-info";
import { ConnectionPicker } from "./picker";
export function ConnectionFields({
  form,
  manager,
  update,
  createIdentity,
  createBastion,
}: {
  form: ManagedSSHConnection;
  manager: SSHManager;
  update: (key: keyof ManagedSSHConnection, value: string | number) => void;
  createIdentity?: () => void;
  createBastion?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <fieldset disabled={manager.busy} className="space-y-5 min-w-0">
      <legend className="text-sm font-medium">{t("settings:sshManager.target")}</legend>
      <div className="space-y-1">
        <Label htmlFor="ssh-connection-name">{t("settings:sshManager.label")}</Label>
        <Input
          id="ssh-connection-name"
          className={settingsControlClassName()}
          value={form.name}
          onChange={(e) => update("name", e.target.value)}
          maxLength={200}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-[1fr_6rem]">
        <div className="space-y-1 min-w-0">
          <Label htmlFor="ssh-managed-host">{t("settings:sshManager.host")}</Label>
          <Input
            id="ssh-managed-host"
            className={settingsControlClassName()}
            value={form.host}
            onChange={(e) => update("host", e.target.value)}
            maxLength={253}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="ssh-managed-port">{t("settings:sshManager.port")}</Label>
          <Input
            id="ssh-managed-port"
            className={settingsControlClassName()}
            type="number"
            min={1}
            max={65535}
            value={form.port}
            onChange={(e) => update("port", Number(e.target.value))}
          />
        </div>
      </div>
      <div className="space-y-1">
        <Label htmlFor="ssh-managed-user">{t("settings:sshManager.user")}</Label>
        <Input
          id="ssh-managed-user"
          className={settingsControlClassName()}
          value={form.user}
          onChange={(e) => update("user", e.target.value)}
          maxLength={256}
        />
      </div>
      <ConnectionPicker
        id="ssh-managed-identity"
        label={t("settings:sshManager.identity")}
        value={form.identity_id}
        onChange={(value) => update("identity_id", value)}
        disabled={manager.busy}
        options={[
          { value: "", label: t("settings:sshManager.selectIdentity") },
          ...manager.identities.map((identity) => ({ value: identity.id, label: identity.label })),
        ]}
      />
      {createIdentity && (
        <CreateIdentityAction
          disabled={manager.busy || !form.name.trim()}
          onClick={createIdentity}
        />
      )}
      <BastionField form={form} manager={manager} update={update} createBastion={createBastion} />
    </fieldset>
  );
}

function CreateIdentityAction({ disabled, onClick }: { disabled: boolean; onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <Button
        type="button"
        variant="link"
        className={settingsActionClassName("px-0")}
        disabled={disabled}
        onClick={onClick}
      >
        {t("settings:sshManager.createIdentityForConnection")}
      </Button>
      <SettingsInfo label={t("settings:sshManager.createIdentityForConnection")}>
        <p>{t("settings:sshManager.ui.createHelp")}</p>
      </SettingsInfo>
    </div>
  );
}

function BastionField({
  form,
  manager,
  update,
  createBastion,
}: Pick<Parameters<typeof ConnectionFields>[0], "form" | "manager" | "update" | "createBastion">) {
  const { t } = useTranslation();
  return (
    <details open={form.bastion_id ? true : undefined} className="min-w-0">
      <summary className="inline-flex items-center cursor-pointer text-sm text-muted-foreground min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
        {t("settings:sshManager.ui.bastionOption")}
      </summary>
      <div className="pt-3">
        <ConnectionPicker
          id="ssh-managed-bastion"
          label={t("settings:sshManager.bastion")}
          value={form.bastion_id ?? ""}
          onChange={(value) => update("bastion_id", value)}
          disabled={manager.busy}
          options={[
            { value: "", label: t("settings:sshManager.direct") },
            ...manager.routes
              .filter(
                (r) =>
                  r.destination.id !== form.id &&
                  !r.destination.bastion_id &&
                  r.destination.verified_route === r.stamp,
              )
              .map((r) => ({ value: r.destination.id, label: r.destination.name })),
          ]}
        />
        {createBastion && (
          <Button
            type="button"
            variant="link"
            className={settingsActionClassName("mt-3 px-0")}
            disabled={manager.busy || !form.name.trim()}
            onClick={createBastion}
          >
            {t("settings:sshManager.createBastion")}
          </Button>
        )}
      </div>
    </details>
  );
}
