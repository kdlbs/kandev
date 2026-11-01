"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { McpSectionHeading } from "@/components/mcp/mcp-origin-sections";
import { McpDefinitionDetails } from "@/components/mcp/mcp-definition-details";
import { McpPendingRowAnnotation } from "./mcp-selection-changes-view";
import type { MCPSetupChange, MCPServerDefinition } from "@/lib/types/http-mcp";

export function McpSessionExclusionAction({
  excluded,
  definition,
  disabled,
  onChange,
}: {
  excluded: boolean;
  definition: MCPServerDefinition;
  disabled?: boolean;
  onChange: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mt-3 flex min-w-0 items-center justify-between gap-3 max-md:flex-col max-md:items-stretch">
      <div className="min-w-0 text-pretty text-xs text-muted-foreground">
        <p>{t(excluded ? "settings:mcpSessionExcluded" : "settings:mcpSessionInherited")}</p>
        {!definition.enabled && (
          <p className="mt-1">{t("settings:mcpRestoreDisabledDescription")}</p>
        )}
      </div>
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        className="h-auto min-h-7 shrink-0 cursor-pointer whitespace-normal py-1 max-md:min-h-11 max-md:w-full [@media(pointer:coarse)]:min-h-11"
        onClick={onChange}
      >
        {t(excluded ? "settings:mcpRestoreInheritance" : "settings:mcpExcludeForSession")}
      </Button>
    </div>
  );
}

export function McpDormantExclusions({
  definitions,
  excludedIds,
  onChange,
  disabled,
}: {
  definitions: MCPServerDefinition[];
  excludedIds: string[];
  onChange: (ids: string[]) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  if (definitions.length === 0) return null;
  return (
    <section className="space-y-3" data-testid="mcp-dormant-exclusions">
      <McpSectionHeading
        title={t("settings:mcpSavedExclusions")}
        description={t("settings:mcpDormantExclusionsDescription")}
        count={definitions.length}
      />
      <div className="divide-y divide-border/60 rounded-lg border border-border/70">
        {definitions.map((definition) => (
          <div
            key={definition.id}
            className="min-w-0 px-3 py-3"
            data-testid={`mcp-dormant-exclusion-${definition.id}`}
          >
            <p className="wrap-anywhere text-[13px] font-medium">{definition.display_name}</p>
            <McpDefinitionDetails definition={definition} />
            <p className="mt-1 text-xs text-muted-foreground">
              {t("settings:mcpNoLongerInherited")}
            </p>
            <McpSessionExclusionAction
              excluded
              definition={definition}
              disabled={disabled}
              onChange={() => onChange(excludedIds.filter((id) => id !== definition.id))}
            />
          </div>
        ))}
      </div>
    </section>
  );
}

export function McpSessionRowAnnotation({
  change,
  definition,
  excludedIds,
  onChange,
  disabled,
}: {
  change?: MCPSetupChange;
  definition?: MCPServerDefinition;
  excludedIds: string[];
  onChange?: (ids: string[]) => void;
  disabled?: boolean;
}) {
  const excluded = Boolean(definition && excludedIds.includes(definition.id));
  return (
    <>
      {change && <McpPendingRowAnnotation change={change} />}
      {onChange && definition && (
        <McpSessionExclusionAction
          definition={definition}
          excluded={excluded}
          disabled={disabled}
          onChange={() =>
            onChange(
              excluded
                ? excludedIds.filter((value) => value !== definition.id)
                : [...excludedIds, definition.id],
            )
          }
        />
      )}
    </>
  );
}
