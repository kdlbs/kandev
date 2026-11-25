import type { SelectConfigOption } from "@/components/model-config-selector";

const PI_THINKING_ID = "thought_level";

type ThinkingProfile = { mode: string; config_options?: Record<string, string> };

export function thinkingProfilePatch(profile: ThinkingProfile, configID: string, value: string) {
  const options = { ...profile.config_options };
  delete options[configID];
  delete options[PI_THINKING_ID];
  if (value) options[PI_THINKING_ID] = value;
  return { mode: "", config_options: options };
}

export function thinkingProfileValue(profile: ThinkingProfile, configID: string) {
  return (
    profile.config_options?.[configID] || profile.config_options?.[PI_THINKING_ID] || profile.mode
  );
}

export function thinkingChoices(
  options: SelectConfigOption[],
  legacy: { id: string; name: string }[],
) {
  if (options.length === 0) return legacy;
  const thinking = options.filter((option) => option.category === "thought_level");
  if (thinking.length !== 1) return [];
  return thinking[0].options.map((choice) => ({ id: choice.value, name: choice.name }));
}
