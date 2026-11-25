"use client";
import { useTranslation } from "react-i18next";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import {
  useConnectionDraft,
  useCreateRelated,
} from "@/hooks/domains/settings/use-ssh-connection-draft";
import { useReturnedIdentity } from "./use-returned-identity";
import { SettingsPageTemplate } from "../settings-page-template";
import { SettingsGroup } from "../settings-group";
import { ConnectionFields } from "./connection-fields";

export function ConnectionDetails({
  route,
  manager,
  onSaved,
  onCreateBastion,
  onCreateIdentity,
}: {
  route?: SSHRoute;
  manager: SSHManager;
  onSaved: (route: SSHRoute) => void;
  onDeleted: () => void;
  onCreateBastion?: (route: SSHRoute) => void;
  onCreateIdentity?: (route: SSHRoute) => void;
}) {
  const { t } = useTranslation();
  const { form, requestId, dirty, update, saveDraft, discard } = useConnectionDraft(route, manager);
  const createRelated = useCreateRelated(saveDraft);
  const unavailableIdentity = useReturnedIdentity(manager.identities, (id) =>
    update("identity_id", id),
  );
  return (
    <SettingsPageTemplate
      title={t("settings:sshManager.connections")}
      showPageChrome={false}
      contentFrame="none"
      saveId={`ssh-connection:${form.id || requestId}`}
      saveRevision={JSON.stringify(form)}
      isDirty={dirty}
      canSave={Boolean(form.name.trim()) && !manager.busy}
      saveStatus={manager.busy ? "loading" : "idle"}
      onSave={async () => onSaved(await saveDraft())}
      onDiscard={discard}
    >
      <SettingsGroup title={t("settings:sshManager.ui.connectionDetails")}>
        <div className="py-4">
          <ConnectionFields
            form={form}
            manager={manager}
            update={update}
            createIdentity={onCreateIdentity ? () => createRelated(onCreateIdentity) : undefined}
            createBastion={onCreateBastion ? () => createRelated(onCreateBastion) : undefined}
          />
          {unavailableIdentity && (
            <p role="alert" className="mt-3 text-sm text-destructive">
              {t("settings:sshManager.recordUnavailable")}
            </p>
          )}
        </div>
      </SettingsGroup>
    </SettingsPageTemplate>
  );
}
