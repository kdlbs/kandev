export type SlashCommandAttrs = {
  id?: unknown;
  label?: unknown;
  commandName?: unknown;
  description?: unknown;
};

function normalizeCommandName(commandName: string): string {
  return commandName.trim().replace(/^\/+/, "");
}

function withLeadingSlash(value: string): string {
  const normalized = normalizeCommandName(value);
  return normalized ? `/${normalized}` : "";
}

export function formatSlashCommandLabel(attrs: SlashCommandAttrs | null | undefined): string {
  if (typeof attrs?.label === "string" && attrs.label.trim()) {
    return withLeadingSlash(attrs.label);
  }
  if (typeof attrs?.commandName === "string" && attrs.commandName.trim()) {
    return withLeadingSlash(attrs.commandName);
  }
  return "";
}

export function formatSlashCommandDisplayLabel(
  attrs: SlashCommandAttrs | null | undefined,
): string {
  return formatSlashCommandLabel(attrs).replace(/^\/+/, "");
}

export function slashCommandHtmlAttributes(
  attrs: SlashCommandAttrs | null | undefined,
): Record<string, string> {
  const htmlAttrs: Record<string, string> = {};
  if (typeof attrs?.id === "string" && attrs.id) {
    htmlAttrs["data-id"] = attrs.id;
  }
  if (typeof attrs?.label === "string" && attrs.label) {
    htmlAttrs["data-label"] = attrs.label;
  }
  if (typeof attrs?.commandName === "string" && attrs.commandName) {
    htmlAttrs["data-command-name"] = attrs.commandName;
  }
  if (typeof attrs?.description === "string" && attrs.description) {
    htmlAttrs["data-description"] = attrs.description;
  }
  return htmlAttrs;
}

export function slashCommandAttrsFromElement(element: Element): SlashCommandAttrs {
  return {
    id: element.getAttribute("data-id"),
    label: element.getAttribute("data-label"),
    commandName: element.getAttribute("data-command-name"),
    description: element.getAttribute("data-description"),
  };
}
