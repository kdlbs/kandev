import { useTranslation } from "react-i18next";
import { ModelConfigSelector, type SelectConfigOption } from "@/components/model-config-selector";
import type { ProfileFormData } from "./profile-form-fields";
import { thinkingChoices, thinkingProfilePatch, thinkingProfileValue } from "./profile-thinking";

// i18n-exempt: local selector identity, never sent to a provider.
const PROVIDER_DEFAULT = "__provider_default__";

export function ProfileThinkingPicker({
  profile,
  options,
  modes,
  onChange,
  disabled,
}: {
  profile: ProfileFormData;
  options: SelectConfigOption[];
  modes: { id: string; name: string }[];
  onChange: (patch: Partial<ProfileFormData>) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const thinking = options.filter((option) => option.category === "thought_level");
  const option = thinking.length === 1 ? thinking[0] : undefined;
  const configID = option?.id ?? "thought_level";
  const value = thinkingProfileValue(profile, configID);
  const advertised = thinkingChoices(options, modes);
  const unavailable = Boolean(value && !advertised.some((choice) => choice.id === value));
  const choices = [{ id: PROVIDER_DEFAULT, name: t("agents:useProviderDefault") }, ...advertised];
  if (unavailable)
    choices.push({ id: value, name: t("agents:thinkingLevelUnavailable", { value }) });
  return (
    <div data-testid="profile-thinking-field" className="space-y-1.5">
      <ModelConfigSelector
        modelOptions={choices.map((choice) => ({
          ...choice,
          disabled: unavailable && choice.id === value,
        }))}
        currentModel={value || PROVIDER_DEFAULT}
        onModelChange={(selected) =>
          onChange(
            thinkingProfilePatch(profile, configID, selected === PROVIDER_DEFAULT ? "" : selected),
          )
        }
        ariaLabel={t("agents:thinkingLevel")}
        pickerTitle={t("agents:thinkingLevel")}
        disabled={disabled}
        popoverAlign="start"
      />
      {unavailable && !disabled ? (
        <p className="text-xs text-destructive">
          {t("agents:thinkingLevelUnavailable", { value })}
        </p>
      ) : null}
    </div>
  );
}
