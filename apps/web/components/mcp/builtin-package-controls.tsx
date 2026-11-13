import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { BuiltinMCPDescriptor } from "@/lib/types/builtin-mcp";
import type { BuiltinMCPEditor } from "./builtin-package-detail";
import { builtinPackageChoice } from "./builtin-package-choice";
import { BuiltinPackageSwitch } from "./builtin-package-switch";

type Props = { editor: BuiltinMCPEditor; item: BuiltinMCPDescriptor };

export function TaskPackageSwitch({
  editor,
  item,
  compact = false,
}: Props & { compact?: boolean }) {
  const { t } = useTranslation();
  const state = builtinPackageChoice(editor.view, editor.draft, "tasks", item.id);
  return (
    <BuiltinPackageSwitch
      label={t(compact ? item.label_key : "settings:builtinMcp.taskChoice")}
      hideLabel={compact}
      checked={state.enabled}
      disabled={
        !editor.view!.can_edit ||
        editor.pending ||
        editor.view!.policy.packages[item.id] === "blocked"
      }
      onCheckedChange={(checked) => editor.setChoice(item.id, checked ? "on" : "off")}
      testId={`builtin-mcp-choice-${item.id}`}
      dirty={state.dirty}
      describedBy={`builtin-mcp-source-${item.id}`}
    />
  );
}

export function TaskPackageInheritance({ editor, item }: Props) {
  const { t } = useTranslation();
  const state = builtinPackageChoice(editor.view, editor.draft, "tasks", item.id);
  const sourceId = `builtin-mcp-source-${item.id}`;
  const defaultValue = t(
    state.inheritedEnabled ? "settings:builtinMcp.choices.on" : "settings:builtinMcp.choices.off",
  );
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      <p id={sourceId} data-testid={sourceId} className="text-xs text-muted-foreground">
        {state.inherited
          ? t("settings:builtinMcp.inheritedValue", { value: defaultValue })
          : t("settings:builtinMcp.overrideValue", {
              value: t(
                state.choice === "on"
                  ? "settings:builtinMcp.choices.on"
                  : "settings:builtinMcp.choices.off",
              ),
              default: defaultValue,
            })}
      </p>
      {!state.inherited && (
        <Button
          type="button"
          variant="link"
          className={controlSizingClassName("standard", "px-0 text-xs")}
          data-testid={`builtin-mcp-inherit-${item.id}`}
          disabled={!editor.view!.can_edit || editor.pending}
          onClick={() => editor.setChoice(item.id, "inherit")}
        >
          {t("settings:builtinMcp.useInherited")}
        </Button>
      )}
    </div>
  );
}

export function TaskPackageControls({ editor, item }: Props) {
  const { t } = useTranslation();
  const state = builtinPackageChoice(editor.view, editor.draft, "tasks", item.id);
  const blocked = editor.view!.policy.packages[item.id] === "blocked";
  return (
    <>
      <TaskPackageSwitch editor={editor} item={item} />
      <TaskPackageInheritance editor={editor} item={item} />
      <p className="text-xs text-muted-foreground" data-testid="builtin-mcp-effective">
        {t(state.enabled ? "settings:builtinMcp.effectiveOn" : "settings:builtinMcp.effectiveOff")}{" "}
        {blocked && t("settings:builtinMcp.blocked")}
      </p>
    </>
  );
}

export function WorkspacePackageControls({ editor, item }: Props) {
  const { t } = useTranslation();
  const state = builtinPackageChoice(editor.view, editor.draft, "workspaces", item.id);
  const allowed = state.choice !== "blocked";
  const disabled = !editor.view!.can_edit || editor.pending;
  return (
    <>
      <BuiltinPackageSwitch
        label={t("settings:builtinMcp.allowTasks")}
        checked={allowed}
        disabled={disabled}
        onCheckedChange={(checked) => editor.setChoice(item.id, checked ? "available" : "blocked")}
        testId={`builtin-mcp-choice-${item.id}`}
        dirty={state.dirty}
      />
      {item.id !== "configuration" && (
        <BuiltinPackageSwitch
          label={t("settings:builtinMcp.choices.default")}
          checked={state.enabled}
          disabled={disabled || !allowed}
          onCheckedChange={(checked) =>
            editor.setChoice(item.id, checked ? "default" : "available")
          }
          testId={`builtin-mcp-default-${item.id}`}
          dirty={state.dirty}
        />
      )}
    </>
  );
}
