"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import type { SSHIdentity } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { SettingsPageTemplate } from "../settings-page-template";
import { SettingsGroup, SettingsRow } from "../settings-group";
import { settingsControlClassName } from "../settings-control";
import { IdentityDependencies } from "./identity-dependencies";
import { PublicKeyDetails } from "./public-key-details";

export function IdentityDetails({
  identity,
  manager,
  editing = false,
  onSaved,
}: {
  identity: SSHIdentity;
  manager: SSHManager;
  editing?: boolean;
  onDeleted: () => void;
  onSaved?: () => void;
}) {
  const { t } = useTranslation();
  const [label, setLabel] = useState(identity.label);
  const value = (text: string) => <span className="text-sm break-all text-right">{text}</span>;
  return (
    <SettingsPageTemplate
      title={identity.label}
      showPageChrome={false}
      contentFrame="none"
      showSaveButton={editing}
      isDirty={label !== identity.label}
      saveId={`ssh-identity:${identity.id}`}
      saveRevision={`${identity.version}:${label}`}
      saveStatus={manager.busy ? "loading" : "idle"}
      canSave={Boolean(label.trim()) && !manager.busy}
      onSave={async () => {
        await manager.renameIdentity(identity, label);
        onSaved?.();
      }}
      onDiscard={() => setLabel(identity.label)}
    >
      <div className="space-y-8">
        <SettingsGroup
          title={t("settings:sshManager.identity")}
          description={t(
            identity.kind === "managed"
              ? "settings:sshManager.custody"
              : "settings:sshManager.externalIdentity",
          )}
        >
          <SettingsRow
            label={t("settings:sshManager.label")}
            controlId={editing ? "ssh-identity-name" : undefined}
            control={
              editing ? (
                <Input
                  id="ssh-identity-name"
                  className={settingsControlClassName()}
                  value={label}
                  onChange={(e) => setLabel(e.target.value)}
                  maxLength={200}
                  disabled={manager.busy}
                />
              ) : (
                value(identity.label)
              )
            }
          />
          {identity.reference && (
            <SettingsRow
              label={t("settings:sshManager.reference")}
              control={value(identity.reference)}
            />
          )}
        </SettingsGroup>
        {!editing && identity.public_key && (
          <SettingsGroup title={t("settings:sshManager.publicKey")}>
            <div className="py-4">
              <PublicKeyDetails publicKey={identity.public_key} />
            </div>
          </SettingsGroup>
        )}
        {!editing && <IdentityDependencies dependencies={identity.dependencies ?? []} />}
      </div>
    </SettingsPageTemplate>
  );
}
