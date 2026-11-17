import type {
  MCPMarketplaceChoice,
  MCPMarketplaceInput,
  MCPSecretBinding,
} from "@/lib/types/http-mcp";

export type MCPValueRow = { name: string; value: string };

export function mcpValueRows(values: Record<string, string> = {}): MCPValueRow[] {
  return Object.entries(values).map(([name, value]) => ({ name, value }));
}

export function mcpValueRecord(
  rows: MCPValueRow[],
  bindings: MCPSecretBinding[] = [],
): Record<string, string> {
  const bound = new Set(bindings.map((binding) => binding.input_name));
  return Object.fromEntries(
    rows
      .filter((row) => !bound.has(row.name))
      .map((row): [string, string] => [row.name, row.value]),
  );
}

export function validMCPInputNames(names: string[]): boolean {
  return (
    names.every((name) => name.length > 0 && name === name.trim()) &&
    new Set(names).size === names.length
  );
}

export function marketplaceInputs(choice: MCPMarketplaceChoice | null): MCPMarketplaceInput[] {
  if (!choice) return [];
  return [...(choice.environment_variables ?? []), ...(choice.headers ?? [])];
}

export function marketplaceInputsComplete(
  choice: MCPMarketplaceChoice | null,
  values: Record<string, string>,
  bindings: MCPSecretBinding[],
): boolean {
  return marketplaceInputs(choice).every((input) => {
    if (!input.isRequired) return true;
    if (!input.name) return false;
    return input.isSecret
      ? bindings.some((binding) => binding.input_name === input.name && Boolean(binding.secret_id))
      : Boolean((values[input.name] ?? input.value)?.trim());
  });
}
