"use client";

import {
  IconArrowDown,
  IconArrowLeft,
  IconArrowRight,
  IconArrowUp,
  IconLayoutColumns,
  IconLayoutRows,
  IconMinus,
  IconPlus,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { DockviewApi, DockviewGroupPanel, IDockviewPanel } from "dockview-react";
import { PANEL_REGISTRY, REUSABLE_PANEL_IDS } from "@/lib/state/layout-manager";
import {
  addReusablePanel,
  mergeGroup,
  moveGroup,
  movePanelToGroup,
  removeReusablePanel,
  reorderTab,
  resizeGroup,
  splitPanel,
} from "./layout-editor-actions";

export type LayoutEditorActionAnchor = {
  left: number;
  top: number;
  width: number;
};

type LayoutEditorActionsProps = {
  api: DockviewApi | null;
  anchor: LayoutEditorActionAnchor | null;
  editable: boolean;
  selectedPanelId: string | null;
  onCommand: () => void;
};

type ActionState = LayoutEditorActionsProps & {
  selected: IDockviewPanel | undefined;
  targetGroups: DockviewGroupPanel[];
  disabled: boolean;
  perform: (command: () => boolean) => void;
};

const touchButtonClass = "min-h-11 min-w-11 cursor-pointer sm:min-h-8 sm:min-w-8";
const directions = [
  { direction: "left" as const, label: "Left", icon: IconArrowLeft },
  { direction: "right" as const, label: "Right", icon: IconArrowRight },
  { direction: "above" as const, label: "Above", icon: IconArrowUp },
  { direction: "below" as const, label: "Below", icon: IconArrowDown },
];

function ActionTooltip({
  label,
  help,
  disabled,
  onClick,
  children,
}: {
  label: string;
  help: string;
  disabled: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={disabled ? 0 : -1} className="inline-flex">
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            className={touchButtonClass}
            disabled={disabled}
            aria-label={label}
            onClick={onClick}
          >
            {children}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{help}</TooltipContent>
    </Tooltip>
  );
}

function MenuTrigger({
  label,
  help,
  disabled,
  children,
}: {
  label: string;
  help: string;
  disabled: boolean;
  children: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={disabled ? 0 : -1} className="inline-flex">
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              className={touchButtonClass}
              disabled={disabled}
              aria-label={label}
            >
              {children}
            </Button>
          </DropdownMenuTrigger>
        </span>
      </TooltipTrigger>
      <TooltipContent>{help}</TooltipContent>
    </Tooltip>
  );
}

function AddPanelAction({ state }: { state: ActionState }) {
  const missing = REUSABLE_PANEL_IDS.filter((id) => !state.api?.getPanel(id));
  const disabled = !state.editable || !state.api || missing.length === 0;
  return (
    <div className="absolute bottom-2 left-2 z-20" data-testid="layout-editor-add-panel">
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={disabled ? 0 : -1} className="inline-flex">
              <DropdownMenuTrigger asChild>
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  className="min-h-11 cursor-pointer shadow-md sm:min-h-8"
                  disabled={disabled}
                  aria-label="Add panel"
                >
                  <IconPlus className="mr-1.5 h-4 w-4" /> Add panel
                </Button>
              </DropdownMenuTrigger>
            </span>
          </TooltipTrigger>
          <TooltipContent>Add a missing tool tab beside the selected split.</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="start">
          {missing.map((id) => (
            <DropdownMenuItem
              key={id}
              className="cursor-pointer"
              title={`Add the ${PANEL_REGISTRY[id].title} tab to the selected split.`}
              onSelect={() =>
                state.perform(() => addReusablePanel(state.api!, id, state.selected?.group.id))
              }
            >
              {PANEL_REGISTRY[id].title}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function SplitMenu({ state }: { state: ActionState }) {
  const disabled = state.disabled || (state.selected?.group.panels.length ?? 0) < 2;
  return (
    <DropdownMenu>
      <MenuTrigger
        label="Split tab"
        help="Move this tab into a new split beside its current split."
        disabled={disabled}
      >
        <IconLayoutColumns className="h-4 w-4" />
      </MenuTrigger>
      <DropdownMenuContent align="start">
        {directions.map(({ direction, label, icon: Icon }) => (
          <DropdownMenuItem
            key={direction}
            className="cursor-pointer"
            title={`Create a new split ${direction} of the selected split.`}
            onSelect={() =>
              state.perform(() => splitPanel(state.api!, state.selectedPanelId!, direction))
            }
          >
            <Icon className="mr-2 h-4 w-4" /> {label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function MovePanelMenu({ state }: { state: ActionState }) {
  return (
    <DropdownMenu>
      <MenuTrigger
        label="Move tab to split"
        help="Move this tab into another existing split."
        disabled={state.disabled || state.targetGroups.length === 0}
      >
        <IconArrowRight className="h-4 w-4" />
      </MenuTrigger>
      <DropdownMenuContent align="start">
        {state.targetGroups.map((group) => (
          <DropdownMenuItem
            key={group.id}
            className="cursor-pointer"
            title={`Move this tab into the split containing ${group.activePanel?.title ?? group.id}.`}
            onSelect={() =>
              state.perform(() => movePanelToGroup(state.api!, state.selectedPanelId!, group.id))
            }
          >
            {group.activePanel?.title ?? group.id}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ArrangeSplitMenu({ state }: { state: ActionState }) {
  const resize = (axis: "width" | "height", delta: number) =>
    state.perform(() => resizeGroup(state.api!, state.selected!.group.id, axis, delta));
  return (
    <DropdownMenu>
      <MenuTrigger
        label="Arrange split"
        help="Resize, reposition, or merge the selected split."
        disabled={state.disabled}
      >
        <IconLayoutRows className="h-4 w-4" />
      </MenuTrigger>
      <DropdownMenuContent align="end" className="max-h-72 overflow-y-auto">
        <DropdownMenuItem
          className="cursor-pointer"
          title="Make the selected split 40 pixels narrower."
          onSelect={() => resize("width", -40)}
        >
          <IconLayoutColumns className="mr-2 h-4 w-4" />
          <IconMinus className="mr-2 h-3 w-3" /> Decrease width
        </DropdownMenuItem>
        <DropdownMenuItem
          className="cursor-pointer"
          title="Make the selected split 40 pixels wider."
          onSelect={() => resize("width", 40)}
        >
          <IconLayoutColumns className="mr-2 h-4 w-4" />
          <IconPlus className="mr-2 h-3 w-3" /> Increase width
        </DropdownMenuItem>
        <DropdownMenuItem
          className="cursor-pointer"
          title="Make the selected split 40 pixels shorter."
          onSelect={() => resize("height", -40)}
        >
          <IconLayoutRows className="mr-2 h-4 w-4" />
          <IconMinus className="mr-2 h-3 w-3" /> Decrease height
        </DropdownMenuItem>
        <DropdownMenuItem
          className="cursor-pointer"
          title="Make the selected split 40 pixels taller."
          onSelect={() => resize("height", 40)}
        >
          <IconLayoutRows className="mr-2 h-4 w-4" />
          <IconPlus className="mr-2 h-3 w-3" /> Increase height
        </DropdownMenuItem>
        {state.targetGroups.length > 0 && <DropdownMenuSeparator />}
        {state.targetGroups.flatMap((target) => [
          ...directions.map(({ direction, label, icon: Icon }) => (
            <DropdownMenuItem
              key={`${target.id}-${direction}`}
              className="cursor-pointer"
              title={`Move this split ${direction} of ${target.activePanel?.title ?? target.id}.`}
              onSelect={() =>
                state.perform(() =>
                  moveGroup(state.api!, state.selected!.group.id, target.id, direction),
                )
              }
            >
              <Icon className="mr-2 h-4 w-4" /> {label} of {target.activePanel?.title ?? target.id}
            </DropdownMenuItem>
          )),
          <DropdownMenuItem
            key={`${target.id}-merge`}
            className="cursor-pointer"
            title={`Merge this split into the split containing ${target.activePanel?.title ?? target.id}.`}
            onSelect={() =>
              state.perform(() => mergeGroup(state.api!, state.selected!.group.id, target.id))
            }
          >
            Merge with {target.activePanel?.title ?? target.id}
          </DropdownMenuItem>,
        ])}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SelectedPanelActions({ state }: { state: ActionState }) {
  if (!state.anchor || !state.selected) return null;
  const first = state.selected.group.panels[0]?.id === state.selectedPanelId;
  const last = state.selected.group.panels.at(-1)?.id === state.selectedPanelId;
  return (
    <div
      className="absolute z-20 flex items-center gap-0.5 overflow-x-auto rounded-md border bg-popover/95 p-1 text-popover-foreground shadow-md backdrop-blur-sm"
      style={{ left: state.anchor.left, top: state.anchor.top, width: state.anchor.width }}
      data-testid="layout-editor-context-actions"
      data-legacy-testid="layout-editor-toolbar"
      aria-label={`Actions for ${state.selected.title ?? state.selected.id}`}
    >
      <span className="hidden max-w-28 truncate px-2 text-xs font-medium sm:inline">
        {state.selected.title ?? state.selected.id}
      </span>
      <ActionTooltip
        label="Move tab left"
        help="Move this tab one position left within its split."
        disabled={state.disabled || first}
        onClick={() => state.perform(() => reorderTab(state.api!, state.selectedPanelId!, -1))}
      >
        <IconArrowLeft className="h-4 w-4" />
      </ActionTooltip>
      <ActionTooltip
        label="Move tab right"
        help="Move this tab one position right within its split."
        disabled={state.disabled || last}
        onClick={() => state.perform(() => reorderTab(state.api!, state.selectedPanelId!, 1))}
      >
        <IconArrowRight className="h-4 w-4" />
      </ActionTooltip>
      <SplitMenu state={state} />
      <MovePanelMenu state={state} />
      <ArrangeSplitMenu state={state} />
      <ActionTooltip
        label="Remove panel"
        help="Remove this tab from the saved layout."
        disabled={state.disabled || state.selectedPanelId === "chat"}
        onClick={() => state.perform(() => removeReusablePanel(state.api!, state.selectedPanelId!))}
      >
        <IconTrash className="h-4 w-4" />
      </ActionTooltip>
    </div>
  );
}

export function LayoutEditorActions(props: LayoutEditorActionsProps) {
  const selected = props.selectedPanelId ? props.api?.getPanel(props.selectedPanelId) : undefined;
  const perform = (command: () => boolean) => {
    if (command()) props.onCommand();
  };
  const state: ActionState = {
    ...props,
    selected,
    targetGroups: props.api?.groups.filter((group) => group.id !== selected?.group.id) ?? [],
    disabled: !props.editable || !props.api || !selected,
    perform,
  };
  return (
    <>
      <AddPanelAction state={state} />
      <SelectedPanelActions state={state} />
    </>
  );
}
