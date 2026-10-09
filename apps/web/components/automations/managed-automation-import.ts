import { parse } from "yaml";

const MAX_IMPORT_BYTES = 2 * 1024 * 1024;

export type ManagedAutomationImportEntry = {
  sourceIndex: number;
  name: string;
  description: string;
  enabled: boolean;
  sourcePluginId: string;
  sourceInstanceKey: string;
  prompt: string;
  trigger: {
    type: "scheduled";
    enabled: boolean;
    config: Record<string, unknown>;
  };
};

export type ManagedAutomationImport = {
  entries: ManagedAutomationImportEntry[];
  skippedCount: number;
};

export class ManagedAutomationImportError extends Error {
  // i18n-exempt: Parser error codes map to translated UI keys.
  constructor(readonly code: "invalid_document" | "no_managed_automations" | "invalid_schedule") {
    super(code);
    this.name = "ManagedAutomationImportError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function readDestination(value: unknown) {
  if (!isRecord(value)) throw new ManagedAutomationImportError("invalid_schedule");
  const pluginId = value.plugin_id;
  const instanceKey = value.instance_key;
  if (
    typeof pluginId !== "string" ||
    pluginId.trim() === "" ||
    typeof instanceKey !== "string" ||
    instanceKey.trim() === ""
  ) {
    throw new ManagedAutomationImportError("invalid_schedule");
  }
  return { pluginId, instanceKey };
}

function readTrigger(value: unknown): ManagedAutomationImportEntry["trigger"] {
  if (!Array.isArray(value) || value.length !== 1) {
    throw new ManagedAutomationImportError("invalid_schedule");
  }
  const trigger = value[0];
  if (!isRecord(trigger) || trigger.type !== "scheduled" || !isRecord(trigger.config)) {
    throw new ManagedAutomationImportError("invalid_schedule");
  }
  return {
    type: "scheduled",
    enabled: trigger.enabled !== false,
    config: trigger.config,
  };
}

function readManagedEntry(value: unknown, sourceIndex: number): ManagedAutomationImportEntry {
  if (!isRecord(value)) throw new ManagedAutomationImportError("invalid_document");
  if (typeof value.name !== "string" || value.name.trim() === "") {
    throw new ManagedAutomationImportError("invalid_schedule");
  }
  const destination = readDestination(value.managed_destination);

  return {
    sourceIndex,
    name: value.name,
    description: typeof value.description === "string" ? value.description : "",
    enabled: value.enabled !== false,
    sourcePluginId: destination.pluginId,
    sourceInstanceKey: destination.instanceKey,
    prompt: typeof value.prompt === "string" ? value.prompt : "",
    trigger: readTrigger(value.triggers),
  };
}

export function parseManagedAutomationImport(document: string): ManagedAutomationImport {
  if (document.length === 0 || new TextEncoder().encode(document).byteLength > MAX_IMPORT_BYTES) {
    throw new ManagedAutomationImportError("invalid_document");
  }

  let parsed: unknown;
  try {
    parsed = parse(document, { uniqueKeys: true, maxAliasCount: 20, version: "1.2" });
  } catch {
    throw new ManagedAutomationImportError("invalid_document");
  }
  if (
    !isRecord(parsed) ||
    parsed.version !== 1 ||
    parsed.type !== "kandev_automations" ||
    !Array.isArray(parsed.automations)
  ) {
    throw new ManagedAutomationImportError("invalid_document");
  }

  const entries: ManagedAutomationImportEntry[] = [];
  parsed.automations.forEach((automation, sourceIndex) => {
    if (!isRecord(automation)) throw new ManagedAutomationImportError("invalid_document");
    if (automation.task_mode === "managed_conversation") {
      entries.push(readManagedEntry(automation, sourceIndex));
    }
  });
  if (entries.length === 0) throw new ManagedAutomationImportError("no_managed_automations");
  return { entries, skippedCount: parsed.automations.length - entries.length };
}
