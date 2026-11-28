"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  AutomationCredentialChoice,
  PersonalCredentialBindingInput,
} from "@/lib/types/automation";
import type { SecretListItem } from "@/lib/types/http-secrets";

const PROFILE_DEFAULT = "__automation_profile_default__";

type ProfileCredentialField = { key: string; secretId: string };

function PersonalCredentialField({
  profileId,
  field,
  bindings,
  choices,
  secrets,
  options,
  loaded,
  canEdit,
  isMobile,
  isFinePointer,
  onChange,
}: {
  profileId: string;
  field: ProfileCredentialField;
  bindings: PersonalCredentialBindingInput[];
  choices: AutomationCredentialChoice[];
  secrets: SecretListItem[];
  options: ComboboxOption[];
  loaded: boolean;
  canEdit: boolean;
  isMobile: boolean;
  isFinePointer: boolean;
  onChange: (bindings: PersonalCredentialBindingInput[]) => void;
}) {
  const { t } = useTranslation();
  const selected = bindings.find(
    (binding) => binding.profile_id === profileId && binding.key === field.key,
  );
  const persistedChoice = choices.find(
    (choice) =>
      choice.kind === "personal" && choice.profile_id === profileId && choice.key === field.key,
  );
  const currentSecret = secrets.find((secret) => secret.id === selected?.secret_id);
  const fieldOptions = [...options];
  if (selected && !currentSecret) {
    fieldOptions.unshift({
      value: selected.secret_id,
      label: t("automations:credentialSavedSecretUnavailable"),
    });
  }
  if (selected) {
    fieldOptions.unshift({
      value: PROFILE_DEFAULT,
      label: t("automations:credentialUseProfileDefault"),
    });
  }

  return (
    <div
      className="grid min-w-0 gap-1.5 sm:grid-cols-[minmax(8rem,0.7fr)_minmax(0,1fr)] sm:items-center"
      data-testid={`automation-personal-credential-${field.key}`}
    >
      <span className="break-all font-mono text-xs text-muted-foreground">{field.key}</span>
      {canEdit ? (
        <div className="min-w-0 space-y-1">
          <Combobox
            options={fieldOptions}
            value={selected?.secret_id ?? ""}
            onValueChange={(value) => {
              const next = bindings.filter(
                (binding) => binding.profile_id !== profileId || binding.key !== field.key,
              );
              if (value === PROFILE_DEFAULT) {
                onChange(next);
                return;
              }
              onChange([...next, { profile_id: profileId, key: field.key, secret_id: value }]);
            }}
            ariaLabel={t("automations:credentialChoosePersonalSecretFor", { key: field.key })}
            drawerTitle={isMobile ? t("automations:credentialChoosePersonalSecret") : undefined}
            placeholder={t("automations:credentialChoosePersonalSecret")}
            searchPlaceholder={t("automations:credentialSecretSearch")}
            emptyMessage={t("automations:credentialNoGlobalSecrets")}
            touchTarget={!isFinePointer}
            className="w-full"
            triggerClassName="min-h-7 justify-between border border-input bg-background px-3 text-left text-xs"
            testId={`automation-personal-secret-${field.key}`}
            dropdownTestId={`automation-personal-secret-options-${field.key}`}
          />
          {loaded && secrets.length === 0 ? (
            <a
              className="text-xs text-primary underline-offset-4 hover:underline"
              href="/settings/secrets"
            >
              {t("automations:credentialAddGlobalSecret")}
            </a>
          ) : null}
        </div>
      ) : (
        <span className="text-xs text-muted-foreground">
          {selected || persistedChoice
            ? t("automations:credentialPersonalSecretSelected")
            : t("automations:credentialProfileDefault")}
        </span>
      )}
    </div>
  );
}

export function PersonalCredentialFields({
  profileId,
  fields,
  bindings,
  choices,
  secrets,
  loaded,
  canEdit,
  onChange,
}: {
  profileId: string;
  fields: ProfileCredentialField[];
  bindings: PersonalCredentialBindingInput[];
  choices: AutomationCredentialChoice[];
  secrets: SecretListItem[];
  loaded: boolean;
  canEdit: boolean;
  onChange: (bindings: PersonalCredentialBindingInput[]) => void;
}) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const options = useMemo<ComboboxOption[]>(
    () =>
      secrets.map((secret) => ({ value: secret.id, label: secret.name, keywords: [secret.name] })),
    [secrets],
  );
  if (fields.length === 0) return null;

  return (
    <div className="space-y-2" data-testid="automation-personal-credential-fields">
      {fields.map((field) => (
        <PersonalCredentialField
          key={`${profileId}:${field.key}`}
          profileId={profileId}
          field={field}
          bindings={bindings}
          choices={choices}
          secrets={secrets}
          options={options}
          loaded={loaded}
          canEdit={canEdit}
          isMobile={isMobile}
          isFinePointer={isFinePointer}
          onChange={onChange}
        />
      ))}
    </div>
  );
}
