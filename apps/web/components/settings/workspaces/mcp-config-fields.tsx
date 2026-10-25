import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { useWorkspaceSecretOptions } from "@/hooks/domains/settings/use-workspace-secret-options";
import type { MCPValueRow } from "@/components/mcp/mcp-config-inputs";
import type { MCPSecretBinding } from "@/lib/types/http-mcp";
import { MCPSecretSelect } from "./mcp-secret-select";

const buttonCursorClassName = "cursor-pointer";

export function MCPValueFields({
  title,
  values,
  onChange,
}: {
  title: string;
  values: MCPValueRow[];
  onChange: (values: MCPValueRow[]) => void;
}) {
  const { t } = useTranslation();
  return (
    <fieldset className="min-w-0 space-y-3 border-t pt-4">
      <legend className="text-sm font-medium">{title}</legend>
      <p className="text-xs text-muted-foreground">{t("settings:mcpPlainInputsHelp")}</p>
      {values.map((row, index) => (
        <div
          key={index}
          className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end"
        >
          <div className="min-w-0 space-y-1">
            <Label htmlFor={`mcp-input-name-${index}`}>
              {t("settings:mcpInputName", { number: index + 1 })}
            </Label>
            <Input
              id={`mcp-input-name-${index}`}
              value={row.name}
              onChange={(event) =>
                onChange(
                  values.map((item, position) =>
                    position === index ? { ...item, name: event.target.value } : item,
                  ),
                )
              }
            />
          </div>
          <div className="min-w-0 space-y-1">
            <Label htmlFor={`mcp-input-value-${index}`}>
              {t("settings:mcpInputValue", { number: index + 1 })}
            </Label>
            <Input
              id={`mcp-input-value-${index}`}
              value={row.value}
              onChange={(event) =>
                onChange(
                  values.map((item, position) =>
                    position === index ? { ...item, value: event.target.value } : item,
                  ),
                )
              }
            />
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={controlSizingClassName("icon", buttonCursorClassName)}
            aria-label={t("settings:mcpRemoveInput", { number: index + 1 })}
            onClick={() => onChange(values.filter((_, position) => position !== index))}
          >
            <IconTrash className="size-4" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={controlSizingClassName("standard", buttonCursorClassName)}
        onClick={() => onChange([...values, { name: "", value: "" }])}
      >
        <IconPlus className="size-4" />
        {t("settings:mcpAddInput")}
      </Button>
    </fieldset>
  );
}

export function MCPSecretFields({
  workspaceId,
  bindings,
  onChange,
}: {
  workspaceId: string;
  bindings: MCPSecretBinding[];
  onChange: (bindings: MCPSecretBinding[]) => void;
}) {
  const { t } = useTranslation();
  const secrets = useWorkspaceSecretOptions(workspaceId);
  return (
    <fieldset className="min-w-0 space-y-3 border-t pt-4">
      <legend className="text-sm font-medium">{t("settings:mcpSecretBindings")}</legend>
      <p className="text-xs text-muted-foreground">{t("settings:mcpSecretBindingsHelp")}</p>
      {bindings.map((binding, index) => (
        <div
          key={index}
          className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto] sm:items-end"
        >
          <div className="min-w-0 space-y-1">
            <Label htmlFor={`mcp-secret-name-${index}`}>
              {t("settings:mcpInputName", { number: index + 1 })}
            </Label>
            <Input
              id={`mcp-secret-name-${index}`}
              value={binding.input_name}
              onChange={(event) =>
                onChange(
                  bindings.map((item, position) =>
                    position === index ? { ...item, input_name: event.target.value } : item,
                  ),
                )
              }
            />
          </div>
          <div className="min-w-0 space-y-1">
            <Label htmlFor={`mcp-secret-reference-${index}`}>
              {t("settings:mcpSecretReference", { number: index + 1 })}
            </Label>
            <MCPSecretSelect
              id={`mcp-secret-reference-${index}`}
              value={binding.secret_id}
              items={secrets.items}
              loading={secrets.loading}
              onChange={(secret_id) =>
                onChange(
                  bindings.map((item, position) =>
                    position === index ? { ...item, secret_id } : item,
                  ),
                )
              }
            />
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={controlSizingClassName("icon", buttonCursorClassName)}
            aria-label={t("settings:mcpRemoveInput", { number: index + 1 })}
            onClick={() => onChange(bindings.filter((_, position) => position !== index))}
          >
            <IconTrash className="size-4" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={controlSizingClassName("standard", buttonCursorClassName)}
        onClick={() => onChange([...bindings, { input_name: "", secret_id: "" }])}
      >
        <IconPlus className="size-4" />
        {t("settings:mcpAddSecretBinding")}
      </Button>
    </fieldset>
  );
}
