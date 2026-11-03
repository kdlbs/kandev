import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Badge } from "@kandev/ui/badge";
import { useWorkspaceSecretOptions } from "@/hooks/domains/settings/use-workspace-secret-options";
import { marketplaceInputs } from "@/components/mcp/mcp-config-inputs";
import type { MCPMarketplaceChoice, MCPSecretBinding } from "@/lib/types/http-mcp";
import { MCPSecretSelect } from "./mcp-secret-select";

export function MCPMarketplaceInputs({
  workspaceId,
  choice,
  values,
  bindings,
  onValuesChange,
  onBindingsChange,
}: {
  workspaceId: string;
  choice: MCPMarketplaceChoice;
  values: Record<string, string>;
  bindings: MCPSecretBinding[];
  onValuesChange: (values: Record<string, string>) => void;
  onBindingsChange: (bindings: MCPSecretBinding[]) => void;
}) {
  const { t } = useTranslation();
  const secrets = useWorkspaceSecretOptions(workspaceId);
  const inputs = marketplaceInputs(choice);
  if (inputs.length === 0) return null;
  return (
    <fieldset className="min-w-0 space-y-3 border-t pt-4">
      <legend className="text-sm font-medium">{t("settings:mcpRequestedInputs")}</legend>
      <p className="text-xs text-muted-foreground">{t("settings:mcpSecretBindingsHelp")}</p>
      {inputs.map((input, index) => {
        const name = input.name ?? "";
        const id = `mcp-marketplace-input-${index}`;
        return (
          <div key={name} className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <Label htmlFor={id}>{name}</Label>
              {input.isRequired && (
                <Badge variant="secondary">{t("settings:mcpRequiredBadge")}</Badge>
              )}
            </div>
            {input.description && (
              <p className="text-xs text-muted-foreground">{input.description}</p>
            )}
            {input.isSecret ? (
              <MCPSecretSelect
                id={id}
                value={bindings.find((binding) => binding.input_name === name)?.secret_id ?? ""}
                items={secrets.items}
                loading={secrets.loading}
                onChange={(secret_id) =>
                  onBindingsChange([
                    ...bindings.filter((binding) => binding.input_name !== name),
                    ...(secret_id ? [{ input_name: name, secret_id }] : []),
                  ])
                }
              />
            ) : (
              <Input
                id={id}
                value={values[name] ?? input.value ?? ""}
                required={input.isRequired}
                onChange={(event) => onValuesChange({ ...values, [name]: event.target.value })}
              />
            )}
          </div>
        );
      })}
    </fieldset>
  );
}
