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
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { DockviewApi, DockviewGroupPanel, IDockviewPanel } from "dockview-react";
import { PANEL_REGISTRY, REUSABLE_PANEL_IDS } from "@/lib/state/layout-manager";
import {
  activatePanel,
  addReusablePanel,
  mergeGroup,
  moveGroup,
  movePanelToGroup,
  removeReusablePanel,
  reorderTab,
  resizeGroup,
  splitPanel,
  type LayoutEditorDirection,
} from "./layout-editor-actions";

type LayoutEditorToolbarProps = {
  api: DockviewApi | null;
  editable: boolean;
  selectedPanelId: string | null;
  onSelectedPanelChange: (panelId: string) => void;
  onCommand: () => void;
};

type ToolbarState = LayoutEditorToolbarProps & {
  selected: IDockviewPanel | undefined;
  targetGroups: DockviewGroupPanel[];
  disabled: boolean;
  perform: (command: () => boolean) => void;
};

const touchButtonClass = "min-h-11 cursor-pointer sm:min-h-8";
const directions = [
  { direction: "left" as const, icon: IconArrowLeft },
  { direction: "right" as const, icon: IconArrowRight },
  { direction: "above" as const, icon: IconArrowUp },
  { direction: "below" as const, icon: IconArrowDown },
];

function IconCommandButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string;
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
            variant="outline"
            className={touchButtonClass}
            disabled={disabled}
            aria-label={label}
            onClick={onClick}
          >
            {children}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function DirectionButtons({
  label,
  onDirection,
  disabled,
}: {
  label: string;
  onDirection: (direction: LayoutEditorDirection) => void;
  disabled: boolean;
}) {
  return (
    <div className="flex items-center gap-1" aria-label={label}>
      {directions.map(({ direction, icon: Icon }) => (
        <IconCommandButton
          key={direction}
          label={`${label} ${direction}`}
          disabled={disabled}
          onClick={() => onDirection(direction)}
        >
          <Icon className="h-4 w-4" />
        </IconCommandButton>
      ))}
    </div>
  );
}

function ResizeButtons({ state }: { state: ToolbarState }) {
  const resize = (axis: "width" | "height", delta: number) =>
    state.perform(() => resizeGroup(state.api!, state.selected!.group.id, axis, delta));
  const controls = [
    {
      label: "Decrease split width",
      axis: "width" as const,
      delta: -40,
      icon: IconLayoutColumns,
      sign: IconMinus,
    },
    {
      label: "Increase split width",
      axis: "width" as const,
      delta: 40,
      icon: IconLayoutColumns,
      sign: IconPlus,
    },
    {
      label: "Decrease split height",
      axis: "height" as const,
      delta: -40,
      icon: IconLayoutRows,
      sign: IconMinus,
    },
    {
      label: "Increase split height",
      axis: "height" as const,
      delta: 40,
      icon: IconLayoutRows,
      sign: IconPlus,
    },
  ];
  return (
    <div className="flex items-center gap-1" aria-label="Resize split">
      {controls.map(({ label, axis, delta, icon: Icon, sign: Sign }) => (
        <IconCommandButton
          key={label}
          label={label}
          disabled={state.disabled}
          onClick={() => resize(axis, delta)}
        >
          <Icon className="h-4 w-4" />
          <Sign className="h-3 w-3" />
        </IconCommandButton>
      ))}
    </div>
  );
}

function AddPanelMenu({ state }: { state: ToolbarState }) {
  const missing = REUSABLE_PANEL_IDS.filter((id) => !state.api?.getPanel(id));
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className={touchButtonClass}
          disabled={!state.editable || !state.api || missing.length === 0}
        >
          <IconPlus className="mr-1.5 h-4 w-4" /> Add panel
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {missing.map((id) => (
          <DropdownMenuItem
            key={id}
            className="cursor-pointer"
            onSelect={() =>
              state.perform(() => addReusablePanel(state.api!, id, state.selected?.group.id))
            }
          >
            {PANEL_REGISTRY[id].title}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function PanelCommands({ state }: { state: ToolbarState }) {
  const first = state.selected?.group.panels[0]?.id === state.selectedPanelId;
  const last = state.selected?.group.panels.at(-1)?.id === state.selectedPanelId;
  return (
    <>
      <Select
        value={state.selectedPanelId ?? undefined}
        onValueChange={state.onSelectedPanelChange}
        disabled={!state.api}
      >
        <SelectTrigger
          className="min-h-11 w-full min-w-0 cursor-pointer sm:min-h-8 sm:w-44"
          aria-label="Selected panel"
        >
          <SelectValue placeholder="Select panel" />
        </SelectTrigger>
        <SelectContent>
          {state.api?.panels.map((panel) => (
            <SelectItem key={panel.id} value={panel.id}>
              {panel.title ?? panel.id}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className={touchButtonClass}
        disabled={state.disabled}
        onClick={() => state.perform(() => activatePanel(state.api!, state.selectedPanelId!))}
      >
        Set active
      </Button>
      <IconCommandButton
        label="Move tab left"
        disabled={state.disabled || first}
        onClick={() => state.perform(() => reorderTab(state.api!, state.selectedPanelId!, -1))}
      >
        <IconArrowLeft className="h-4 w-4" />
      </IconCommandButton>
      <IconCommandButton
        label="Move tab right"
        disabled={state.disabled || last}
        onClick={() => state.perform(() => reorderTab(state.api!, state.selectedPanelId!, 1))}
      >
        <IconArrowRight className="h-4 w-4" />
      </IconCommandButton>
      <IconCommandButton
        label="Remove panel"
        disabled={state.disabled || state.selectedPanelId === "chat"}
        onClick={() => state.perform(() => removeReusablePanel(state.api!, state.selectedPanelId!))}
      >
        <IconTrash className="h-4 w-4" />
      </IconCommandButton>
    </>
  );
}

function TargetGroupMenus({ state }: { state: ToolbarState }) {
  if (state.targetGroups.length === 0) return null;
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={touchButtonClass}
            disabled={state.disabled}
          >
            Move to
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {state.targetGroups.map((group) => (
            <DropdownMenuItem
              key={group.id}
              className="cursor-pointer"
              onSelect={() =>
                state.perform(() => movePanelToGroup(state.api!, state.selectedPanelId!, group.id))
              }
            >
              {group.activePanel?.title ?? group.id}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={touchButtonClass}
            disabled={state.disabled}
          >
            Split actions
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {state.targetGroups.flatMap((target) => [
            ...directions.map(({ direction }) => (
              <DropdownMenuItem
                key={`${target.id}-${direction}`}
                className="cursor-pointer"
                onSelect={() =>
                  state.perform(() =>
                    moveGroup(state.api!, state.selected!.group.id, target.id, direction),
                  )
                }
              >
                Move split {direction} {target.activePanel?.title ?? target.id}
              </DropdownMenuItem>
            )),
            <DropdownMenuItem
              key={`${target.id}-merge`}
              className="cursor-pointer"
              onSelect={() =>
                state.perform(() => mergeGroup(state.api!, state.selected!.group.id, target.id))
              }
            >
              Merge with {target.activePanel?.title ?? target.id}
            </DropdownMenuItem>,
          ])}
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}

export function LayoutEditorToolbar(props: LayoutEditorToolbarProps) {
  const selected = props.selectedPanelId ? props.api?.getPanel(props.selectedPanelId) : undefined;
  const perform = (command: () => boolean) => {
    if (command()) props.onCommand();
  };
  const state: ToolbarState = {
    ...props,
    selected,
    targetGroups: props.api?.groups.filter((group) => group.id !== selected?.group.id) ?? [],
    disabled: !props.editable || !props.api || !selected,
    perform,
  };
  return (
    <div
      className="flex min-w-0 flex-wrap items-center gap-2 border-b bg-muted/30 p-2"
      data-testid="layout-editor-toolbar"
    >
      <AddPanelMenu state={state} />
      <PanelCommands state={state} />
      <DirectionButtons
        label="Split panel"
        disabled={state.disabled || (selected?.group.panels.length ?? 0) < 2}
        onDirection={(direction) =>
          perform(() => splitPanel(props.api!, props.selectedPanelId!, direction))
        }
      />
      <ResizeButtons state={state} />
      <TargetGroupMenus state={state} />
    </div>
  );
}
