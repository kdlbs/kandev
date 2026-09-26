import { usableConfigOptions, type SelectConfigOption } from "@/components/model-config-selector";
import type { ConfigOptionEntry, ModelConfig } from "@/lib/types/http";

export function reconcileConfigOptionValues(
  values: Record<string, string> | undefined,
  options: ConfigOptionEntry[] | undefined,
): Record<string, string> {
  const optionById = new Map((options ?? []).map((option) => [option.id, option]));
  return Object.entries(values ?? {}).reduce<Record<string, string>>((result, [id, value]) => {
    const option = optionById.get(id);
    if (!option) return result;
    if (option.options?.length && !option.options.some((choice) => choice.value === value)) {
      return result;
    }
    result[id] = value;
    return result;
  }, {});
}

export function modelConfigOptions(modelConfig: ModelConfig): SelectConfigOption[] {
  return usableConfigOptions(
    modelConfig.config_options?.map((option) => ({
      type: option.type,
      id: option.id,
      name: option.name,
      currentValue: option.current_value,
      category: option.category,
      options: option.options,
    })),
  );
}
