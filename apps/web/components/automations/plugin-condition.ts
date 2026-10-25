import type { AutomationTrigger, TriggerTypeInfo } from "@/lib/types/automation";

export function findTriggerInfo(trigger: AutomationTrigger | undefined, infos: TriggerTypeInfo[]) {
  if (!trigger) return infos.find((info) => info.type === "scheduled");
  return infos.find(
    (info) =>
      info.type === trigger.type &&
      (trigger.type !== "plugin_event" ||
        (info.plugin?.plugin_id === trigger.config.plugin_id &&
          info.plugin?.condition.key === trigger.config.condition_key)),
  );
}
export function conditionSettings(config: Record<string, unknown>): Record<string, unknown> {
  const settings = config.settings;
  return settings && typeof settings === "object" && !Array.isArray(settings)
    ? (settings as Record<string, unknown>)
    : {};
}
