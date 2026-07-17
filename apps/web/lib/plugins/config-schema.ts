/**
 * Turns a plugin manifest's `config_schema` (a JSON-Schema-like object the
 * plugin author declares — see apps/backend/internal/plugins/config.go) into
 * renderable form fields for the plugin settings page, and converts form
 * values back into the config payload for PATCH /api/plugins/:id.
 *
 * Secret fields (`secret: true` or `format: "password"`, e.g. a GitHub PAT)
 * come back from GET /api/plugins/:id/config as SECRET_MASK; submitting the
 * mask unchanged tells the backend to keep the stored value.
 */

/** Mirror of the backend's configSecretMask (internal/plugins/config.go). */
export const SECRET_MASK = "********";

export type PluginConfigFieldType = "string" | "boolean" | "number" | "integer" | "enum";

export interface PluginConfigField {
  name: string;
  type: PluginConfigFieldType;
  label: string;
  description?: string;
  required: boolean;
  secret: boolean;
  enumValues?: string[];
  defaultValue?: unknown;
}

type SchemaObject = Record<string, unknown>;

function asObject(value: unknown): SchemaObject | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as SchemaObject)
    : null;
}

function fieldType(prop: SchemaObject): PluginConfigFieldType {
  const enumValues = prop.enum;
  if (Array.isArray(enumValues) && enumValues.length > 0) return "enum";
  const type = prop.type;
  if (type === "boolean" || type === "number" || type === "integer") return type;
  return "string";
}

function isSecretProp(prop: SchemaObject): boolean {
  return prop.secret === true || prop.format === "password";
}

function requiredNames(schema: SchemaObject): Set<string> {
  const required = schema.required;
  if (!Array.isArray(required)) return new Set();
  return new Set(required.filter((name): name is string => typeof name === "string"));
}

/**
 * Parses a config_schema into ordered form fields. Only declared
 * `properties` become fields; richer schema constructs are ignored (the
 * schema is authoring metadata, not a full JSON-Schema contract). Returns []
 * for a missing or unusable schema — the settings page then shows the
 * "no settings" state.
 */
export function parseConfigSchema(
  schema: Record<string, unknown> | undefined,
): PluginConfigField[] {
  const schemaObj = asObject(schema);
  if (!schemaObj) return [];
  const properties = asObject(schemaObj.properties);
  if (!properties) return [];
  const required = requiredNames(schemaObj);

  const fields: PluginConfigField[] = [];
  for (const [name, raw] of Object.entries(properties)) {
    const prop = asObject(raw);
    if (!prop) continue;
    fields.push({
      name,
      type: fieldType(prop),
      label: typeof prop.title === "string" && prop.title !== "" ? prop.title : name,
      description: typeof prop.description === "string" ? prop.description : undefined,
      required: required.has(name),
      secret: isSecretProp(prop),
      enumValues: Array.isArray(prop.enum) ? prop.enum.map((v) => String(v)) : undefined,
      defaultValue: prop.default,
    });
  }
  return fields;
}

/**
 * Builds the form's initial values from the (masked) stored config: stored
 * value wins, then the schema default, then a type-appropriate empty value.
 * Everything non-boolean is kept as a string for the input elements.
 */
export function buildInitialValues(
  fields: PluginConfigField[],
  config: Record<string, unknown>,
): Record<string, string | boolean> {
  const values: Record<string, string | boolean> = {};
  for (const field of fields) {
    const stored = config[field.name] ?? field.defaultValue;
    if (field.type === "boolean") {
      values[field.name] = typeof stored === "boolean" ? stored : false;
    } else {
      values[field.name] = stored === undefined || stored === null ? "" : String(stored);
    }
  }
  return values;
}

function coerceNumeric(field: PluginConfigField, raw: string): number | undefined {
  const parsed = field.type === "integer" ? Number.parseInt(raw, 10) : Number(raw);
  return Number.isNaN(parsed) ? undefined : parsed;
}

/**
 * Converts form values back into the config object to PATCH. Empty string
 * inputs are omitted (an unset optional field stays unset rather than
 * persisting ""), booleans are always included, and numeric fields are
 * parsed — an unparseable numeric input is omitted so the backend's schema
 * validation reports the missing required field instead of a bogus value.
 */
export function serializeConfigValues(
  fields: PluginConfigField[],
  values: Record<string, string | boolean>,
): Record<string, unknown> {
  const config: Record<string, unknown> = {};
  for (const field of fields) {
    const value = values[field.name];
    if (field.type === "boolean") {
      config[field.name] = value === true;
      continue;
    }
    if (typeof value !== "string" || value === "") continue;
    if (field.type === "number" || field.type === "integer") {
      const parsed = coerceNumeric(field, value);
      if (parsed !== undefined) config[field.name] = parsed;
      continue;
    }
    config[field.name] = value;
  }
  return config;
}

/**
 * Client-side pre-validation matching the backend's required-field check, so
 * the form can flag missing values before a round-trip. A secret field
 * showing the mask counts as set (the backend keeps the stored value).
 */
export function missingRequiredFields(
  fields: PluginConfigField[],
  values: Record<string, string | boolean>,
): string[] {
  return fields
    .filter((field) => field.required && field.type !== "boolean")
    .filter((field) => {
      const value = values[field.name];
      return typeof value !== "string" || value.trim() === "";
    })
    .map((field) => field.label);
}
