"use client";

import { useTranslation } from "react-i18next";
import { IconRouter } from "@tabler/icons-react";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { InlineSecretSelect } from "@/components/settings/profile-edit/inline-secret-select";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { isProviderConfigDirty } from "@/components/settings/agent-profile-dirty";
import {
  PROVIDER_KIND_OPENAI_COMPATIBLE,
  isCredentialTransportSafe,
  isOpenAICompatibleProvider,
  isValidProviderBaseUrl,
} from "@/lib/settings/provider-config-validation";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { AgentProfile } from "@/lib/types/http";

const BASE_URL_EXAMPLE = "http://localhost:20128/v1";
const NATIVE_VALUE = "native";

type ProviderSectionProps = {
  draft: AgentProfile;
  savedProfile: AgentProfile;
  secrets: SecretListItem[];
  onChange: (patch: Partial<AgentProfile>) => void;
};

/**
 * OpenAI-compatible provider section of the agent profile editor. Rendered only
 * when the profile's agent advertises provider support (`providerSupported`).
 * Switching back to "Native" clears the base URL and key so the saved payload
 * carries nothing stale.
 */
export function ProviderSection({ draft, savedProfile, secrets, onChange }: ProviderSectionProps) {
  const { t } = useTranslation();
  if (!draft.providerSupported) return null;

  const isCustom = isOpenAICompatibleProvider(draft.providerKind);

  const handleKindChange = (next: string) => {
    if (isOpenAICompatibleProvider(next)) {
      onChange({ providerKind: PROVIDER_KIND_OPENAI_COMPATIBLE });
      return;
    }
    onChange({ providerKind: "", providerBaseUrl: "", providerApiKeySecretId: "" });
  };

  return (
    <SettingsCard isDirty={isProviderConfigDirty(draft, savedProfile)}>
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconRouter className="h-5 w-5" />
            {t("agents:providerSectionTitle")}
          </span>
        }
        description={t("agents:providerSectionDescription")}
      />
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="profile-provider-kind">{t("agents:providerKindLabel")}</Label>
          <Select
            value={isCustom ? PROVIDER_KIND_OPENAI_COMPATIBLE : NATIVE_VALUE}
            onValueChange={handleKindChange}
          >
            <SelectTrigger
              id="profile-provider-kind"
              className="cursor-pointer"
              data-testid="provider-kind-select"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NATIVE_VALUE}>{t("agents:providerKindNative")}</SelectItem>
              <SelectItem value={PROVIDER_KIND_OPENAI_COMPATIBLE}>
                {t("agents:providerKindOpenAICompatible")}
              </SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">{t("agents:providerKindHelp")}</p>
        </div>

        {isCustom && (
          <OpenAICompatibleProviderFields
            draft={draft}
            savedProfile={savedProfile}
            secrets={secrets}
            onChange={onChange}
          />
        )}
      </CardContent>
    </SettingsCard>
  );
}

function OpenAICompatibleProviderFields({
  draft,
  savedProfile,
  secrets,
  onChange,
}: ProviderSectionProps) {
  const { t } = useTranslation();
  const baseUrl = draft.providerBaseUrl ?? "";
  const hasApiKey = (draft.providerApiKeySecretId ?? "").trim() !== "";
  const baseUrlInvalid =
    baseUrl.trim() !== "" &&
    (!isValidProviderBaseUrl(baseUrl) || (hasApiKey && !isCredentialTransportSafe(baseUrl)));
  const modelHasSlash = (draft.model ?? "").includes("/");
  const keyDirty =
    (draft.providerApiKeySecretId ?? "") !== (savedProfile.providerApiKeySecretId ?? "");

  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="profile-provider-base-url">{t("agents:providerBaseUrlLabel")}</Label>
        <Input
          id="profile-provider-base-url"
          data-testid="provider-base-url-input"
          value={baseUrl}
          onChange={(event) => onChange({ providerBaseUrl: event.target.value })}
          placeholder={t("agents:exampleValue", { example: BASE_URL_EXAMPLE })}
          aria-invalid={baseUrlInvalid}
        />
        {baseUrlInvalid && (
          <p className="text-xs text-destructive" data-testid="provider-base-url-error">
            {t("agents:providerBaseUrlInvalid")}
          </p>
        )}
        <p className="text-xs text-muted-foreground">{t("agents:providerBaseUrlHelp")}</p>
      </div>

      <InlineSecretSelect
        secretId={draft.providerApiKeySecretId ?? null}
        onSecretIdChange={(id) => onChange({ providerApiKeySecretId: id ?? "" })}
        secrets={secrets}
        label={t("agents:providerApiKeyLabel")}
        isDirty={keyDirty}
      />
      <p className="text-xs text-muted-foreground">{t("agents:providerApiKeyHelp")}</p>

      {modelHasSlash && (
        <p className="text-xs text-destructive" data-testid="provider-model-error">
          {t("agents:providerModelSlash")}
        </p>
      )}
    </>
  );
}
