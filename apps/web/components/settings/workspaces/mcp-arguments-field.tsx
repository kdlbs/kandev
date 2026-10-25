import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { controlSizingClassName } from "@kandev/ui/control-sizing";

export function MCPArgumentsField({
  values,
  onChange,
}: {
  values: string[];
  onChange: (values: string[]) => void;
}) {
  const { t } = useTranslation();
  return (
    <fieldset className="min-w-0 space-y-2">
      <legend className="text-sm font-medium">{t("settings:mcpArguments")}</legend>
      <p className="text-xs text-muted-foreground">{t("settings:mcpArgumentsHelp")}</p>
      {values.map((value, index) => (
        <div key={index} className="flex min-w-0 items-start gap-2">
          <Label htmlFor={`mcp-argument-${index}`} className="sr-only">
            {t("settings:mcpArgumentLabel", { number: index + 1 })}
          </Label>
          <Input
            id={`mcp-argument-${index}`}
            value={value}
            onChange={(event) =>
              onChange(
                values.map((item, position) => (position === index ? event.target.value : item)),
              )
            }
            className="min-w-0 flex-1"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={controlSizingClassName("icon", "shrink-0 cursor-pointer")}
            aria-label={t("settings:mcpRemoveArgument", { number: index + 1 })}
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
        className={controlSizingClassName("standard", "cursor-pointer")}
        onClick={() => onChange([...values, ""])}
      >
        <IconPlus className="size-4" />
        {t("settings:mcpAddArgument")}
      </Button>
    </fieldset>
  );
}
