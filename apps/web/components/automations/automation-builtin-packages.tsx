import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  BuiltinPackageHeader,
  BuiltinPackageInformation,
  BuiltinPackageRow,
} from "@/components/mcp/builtin-package-content";
import type { BuiltinMCPDescriptor, BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import type {
  AutomationMCPOptions,
  AutomationMCPSelection,
  TaskMode,
} from "@/lib/types/automation";

/** Required Core cannot be removed. Workspace blocks prevent enabling normal-task packages. */
function AutomationPackageChoice({
  item,
  value,
  onChange,
  disabled,
  blocked,
}: {
  item: BuiltinMCPDescriptor;
  value: AutomationMCPSelection;
  onChange: (value: AutomationMCPSelection) => void;
  disabled: boolean;
  blocked: boolean;
}) {
  const { t } = useTranslation();
  const enabled = value.builtin_packages.includes(item.id);
  return (
    <>
      {item.id === "core" ? (
        <p className="rounded-md bg-muted p-3 text-sm">{t("settings:builtinMcp.coreRequired")}</p>
      ) : (
        <div className="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-3">
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <label className="text-sm font-medium" htmlFor={`automation-package-${item.id}`}>
              <span className="sr-only">{t(item.label_key)} </span>
              {t("automations:mcpAccess")}
            </label>
            <Select
              value={enabled ? "on" : "off"}
              disabled={disabled}
              onValueChange={(choice) =>
                onChange({
                  ...value,
                  builtin_packages:
                    choice === "on"
                      ? [...value.builtin_packages.filter((id) => id !== item.id), item.id]
                      : value.builtin_packages.filter((id) => id !== item.id),
                })
              }
            >
              <SelectTrigger
                id={`automation-package-${item.id}`}
                className={controlSizingClassName("standard", "w-full md:w-64")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="on" disabled={blocked}>
                  {t("settings:builtinMcp.choices.on")}
                </SelectItem>
                <SelectItem value="off">{t("settings:builtinMcp.choices.off")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {blocked && (
            <p className="text-xs text-muted-foreground">{t("automations:mcpBlocked")}</p>
          )}
        </div>
      )}
    </>
  );
}

/** Keep automation grants explicit while sharing the task package list and detail presentation. */
export function AutomationBuiltinPackages({
  options,
  value,
  onChange,
  disabled,
  taskMode,
}: {
  options?: AutomationMCPOptions;
  value: AutomationMCPSelection;
  onChange: (value: AutomationMCPSelection) => void;
  disabled: boolean;
  taskMode: TaskMode;
}) {
  const { t } = useTranslation();
  const [selected, select] = useState<BuiltinMCPPackage | null>(null);
  const root = useRef<HTMLFieldSetElement>(null);
  const returnFocus = useRef<BuiltinMCPPackage | null>(null);
  const item = options?.packages.find((pkg) => pkg.id === selected);
  useEffect(() => {
    if (!item && returnFocus.current) {
      root.current
        ?.querySelector<HTMLButtonElement>(
          `[data-testid="automation-mcp-package-${returnFocus.current}"]`,
        )
        ?.focus();
      returnFocus.current = null;
    }
  }, [item]);
  const blocked = item && taskMode === "normal_task" && options?.task_blocked.includes(item.id);
  return (
    <fieldset ref={root} disabled={disabled} className="min-w-0 space-y-3">
      <legend className="text-sm font-medium">{t("automations:mcpBuiltinPackages")}</legend>
      {item ? (
        <section className="min-w-0 space-y-4" data-testid="automation-mcp-package-detail">
          <BuiltinPackageHeader
            item={item}
            onBack={() => select(null)}
            backgroundClassName="bg-card"
          />
          <AutomationPackageChoice
            item={item}
            value={value}
            onChange={onChange}
            disabled={disabled}
            blocked={Boolean(blocked)}
          />
          <BuiltinPackageInformation item={item} />
        </section>
      ) : (
        <div className="divide-y rounded-lg border">
          {options?.packages.map((pkg) => (
            <BuiltinPackageRow
              key={pkg.id}
              item={pkg}
              testId={`automation-mcp-package-${pkg.id}`}
              labelKey={
                pkg.id === "core"
                  ? "settings:builtinMcp.required"
                  : `settings:builtinMcp.choices.${value.builtin_packages.includes(pkg.id) ? "on" : "off"}`
              }
              onSelect={() => {
                returnFocus.current = pkg.id;
                select(pkg.id);
              }}
            />
          ))}
        </div>
      )}
    </fieldset>
  );
}
