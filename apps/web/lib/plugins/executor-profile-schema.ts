export type ExecutorProfileField = {
  name: string;
  label: string;
  description?: string;
  type: "string" | "boolean" | "number" | "integer";
  required: boolean;
  secret: boolean;
  enumValues: string[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  defaultValue?: string;
};

export type ExecutorProfileFieldError = {
  code: "required" | "type" | "enum" | "minimum" | "maximum" | "minLength" | "maxLength";
  value?: number;
};

export type ParsedExecutorProfileSchema = {
  fields: ExecutorProfileField[];
  supported: boolean;
};

export type ExecutorProfileValues = Record<string, string>;
export type ExecutorProfileFieldErrors = Record<string, ExecutorProfileFieldError>;

type SchemaObject = Record<string, unknown>;
type ScalarFieldType = ExecutorProfileField["type"];

function asObject(value: unknown): SchemaObject | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as SchemaObject)
    : null;
}

function isScalar(value: unknown): value is string | number | boolean {
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean";
}

function scalarFieldType(value: unknown): value is ScalarFieldType {
  return value === "string" || value === "boolean" || value === "number" || value === "integer";
}

function requiredNames(value: unknown): Set<string> {
  return new Set(
    Array.isArray(value) ? value.filter((name): name is string => typeof name === "string") : [],
  );
}

function parseField(
  name: string,
  raw: unknown,
  required: Set<string>,
): ExecutorProfileField | null {
  const property = asObject(raw);
  if (!property || !scalarFieldType(property.type)) return null;
  const enumValues = Array.isArray(property.enum) ? property.enum : [];
  if (!enumValues.every(isScalar)) return null;
  return {
    name,
    label: typeof property.title === "string" && property.title ? property.title : name,
    description: typeof property.description === "string" ? property.description : undefined,
    type: property.type,
    required: required.has(name),
    secret: property.secret === true,
    enumValues: enumValues.map(String),
    minimum: typeof property.minimum === "number" ? property.minimum : undefined,
    maximum: typeof property.maximum === "number" ? property.maximum : undefined,
    minLength: typeof property.minLength === "number" ? property.minLength : undefined,
    maxLength: typeof property.maxLength === "number" ? property.maxLength : undefined,
    defaultValue: property.secret === true ? undefined : defaultText(property.default),
  };
}

function defaultText(value: unknown): string | undefined {
  return isScalar(value) ? String(value) : undefined;
}

export function parseExecutorProfileSchema(
  schema: Record<string, unknown> | undefined,
): ParsedExecutorProfileSchema {
  const root = asObject(schema);
  const properties = asObject(root?.properties);
  if (!root || root.type !== "object" || !properties || Object.keys(properties).length === 0) {
    return { fields: [], supported: false };
  }

  const required = requiredNames(root.required);
  const fields = Object.entries(properties).map(([name, raw]) => parseField(name, raw, required));
  if (fields.some((field) => field === null)) return { fields: [], supported: false };
  return { fields: fields as ExecutorProfileField[], supported: true };
}

export function buildExecutorProfileValues(
  fields: ExecutorProfileField[],
  config: Record<string, string> | undefined,
): ExecutorProfileValues {
  return Object.fromEntries(fields.map((field) => [field.name, config?.[field.name] ?? ""]));
}

export function validateExecutorProfileValues(
  fields: ExecutorProfileField[],
  values: ExecutorProfileValues,
  configuredSecrets: Record<string, boolean> = {},
  clearedSecrets: Record<string, boolean> = {},
): ExecutorProfileFieldErrors {
  const errors: ExecutorProfileFieldErrors = {};
  for (const field of fields) {
    const error = validateField(
      field,
      values[field.name] ?? "",
      configuredSecrets[field.name] === true,
      clearedSecrets[field.name] === true,
    );
    if (error) errors[field.name] = error;
  }
  return errors;
}

function validateField(
  field: ExecutorProfileField,
  value: string,
  secretConfigured: boolean,
  secretCleared: boolean,
): ExecutorProfileFieldError | undefined {
  if (!value) {
    if (field.secret && secretConfigured && !secretCleared) return undefined;
    if (field.defaultValue !== undefined) return undefined;
    return field.required ? { code: "required" } : undefined;
  }
  const lengthError = validateLength(field, value);
  if (lengthError) return lengthError;
  if (field.enumValues.length > 0 && !field.enumValues.includes(value)) return { code: "enum" };
  if (field.type === "boolean" && value !== "true" && value !== "false") return { code: "type" };
  if (field.type !== "number" && field.type !== "integer") return undefined;
  return validateNumber(field, value);
}

function validateLength(
  field: ExecutorProfileField,
  value: string,
): ExecutorProfileFieldError | undefined {
  if (field.type !== "string") return undefined;
  const length = Array.from(value).length;
  if (field.minLength !== undefined && length < field.minLength) {
    return { code: "minLength", value: field.minLength };
  }
  if (field.maxLength !== undefined && length > field.maxLength) {
    return { code: "maxLength", value: field.maxLength };
  }
  return undefined;
}

function validateNumber(
  field: ExecutorProfileField,
  value: string,
): ExecutorProfileFieldError | undefined {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || (field.type === "integer" && !Number.isInteger(numeric))) {
    return { code: "type" };
  }
  if (field.minimum !== undefined && numeric < field.minimum) {
    return { code: "minimum", value: field.minimum };
  }
  if (field.maximum !== undefined && numeric > field.maximum) {
    return { code: "maximum", value: field.maximum };
  }
  return undefined;
}

export function serializeExecutorProfileValues(
  fields: ExecutorProfileField[],
  values: ExecutorProfileValues,
  clearedSecrets: Record<string, boolean> = {},
): Record<string, string> {
  const config: Record<string, string> = {};
  for (const field of fields) {
    const value = values[field.name] ?? "";
    if (!field.secret || clearedSecrets[field.name]) {
      config[field.name] = value;
    } else if (value !== "") {
      config[field.name] = value;
    }
  }
  return config;
}
