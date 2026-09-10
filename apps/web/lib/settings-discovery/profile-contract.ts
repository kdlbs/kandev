import contract from "./profile-contract.generated.json";

type ContractField = {
  key: string;
  field_path: string;
  support: string;
  writable?: boolean;
  sensitive?: boolean;
  replacement?: boolean;
};

type ContractDomain = {
  resource_type: string;
  fields: ContractField[];
};

const domains = contract.domains as ContractDomain[];

export const PROFILE_EDITABLE_FIELD_PATHS = [
  "name",
  "model",
  "fallback_model",
  "auto_fallback",
  "mode",
  "config_options",
  "allow_indexing",
  "auto_approve",
  "cli_passthrough",
  "enabled",
  "cli_flags",
  "env_vars",
  "command_prefix",
  "dynamic",
] as const;

export type ProfileEditableFieldPath = (typeof PROFILE_EDITABLE_FIELD_PATHS)[number];

export function profileContractField(
  resourceType: "agent_profile" | "agent_profile_mcp",
  fieldPath: string,
): ContractField | undefined {
  return domains
    .find((domain) => domain.resource_type === resourceType)
    ?.fields.find((field) => field.field_path === fieldPath);
}

export function profileContractFields(): ContractField[] {
  return PROFILE_EDITABLE_FIELD_PATHS.map((fieldPath) =>
    profileContractField("agent_profile", fieldPath),
  ).filter((field): field is ContractField => field !== undefined);
}

export function profileContractCovers(fieldPath: string): fieldPath is ProfileEditableFieldPath {
  return (PROFILE_EDITABLE_FIELD_PATHS as readonly string[]).includes(fieldPath);
}
