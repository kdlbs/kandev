import type {
  AutomationMCPOptions,
  AutomationMCPSelection,
  AutomationPluginToolSelection,
} from "@/lib/types/automation";

type Translate = (key: string, options?: Record<string, unknown>) => string;

/**
 * One line naming the tools an automation's agent can use: enabled built-in
 * packages in catalog order, selected workspace servers, and the number of
 * selected plugin tools, joined for the active language.
 */
export function summarizeAgentTools({
  selection,
  options,
  pluginTools,
  translate,
  language,
}: {
  selection: AutomationMCPSelection;
  options?: AutomationMCPOptions;
  pluginTools: AutomationPluginToolSelection[];
  translate: Translate;
  language: string;
}): string {
  const packages = (options?.packages ?? [])
    .filter((pkg) => selection.builtin_packages.includes(pkg.id))
    .map((pkg) => translate(pkg.label_key));
  const servers = selection.definition_ids.flatMap((id) => {
    const definition = options?.definitions.find((item) => item.id === id);
    return definition ? [definition.name] : [];
  });
  const items = [...packages, ...servers];
  const toolCount = pluginTools.reduce((total, group) => total + group.tool_names.length, 0);
  if (toolCount > 0)
    items.push(translate("automations:toolsSummaryPluginTools", { count: toolCount }));
  return new Intl.ListFormat(language, { style: "long", type: "unit" }).format(items);
}
