"use client";

import type { ReactNode, Ref } from "react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { cn } from "@kandev/ui/utils";
import type {
  PluginActionPresentation,
  PluginActionSurfaceName,
} from "@/components/plugins/plugin-action-surface";
import { surfaceActionClassName, surfaceActionGlyphClassName } from "./surface-action-styles";

const TONE_CLASS: Record<NonNullable<SurfaceActionProps["tone"]>, string> = {
  neutral: "",
  success: "text-emerald-600 dark:text-emerald-400",
  warning: "text-amber-600 dark:text-amber-400",
  danger: "text-destructive",
};

export type SurfaceActionProps = {
  surface: PluginActionSurfaceName;
  presentation?: PluginActionPresentation;
  label?: string;
  icon?: ReactNode;
  text?: string;
  badge?: string;
  children?: ReactNode;
  tone?: "neutral" | "success" | "warning" | "danger";
  pressed?: boolean;
  disabled?: boolean;
  busy?: boolean;
  ref?: Ref<HTMLButtonElement>;
  type?: "button";
  title?: string;
  id?: string;
  "aria-expanded"?: boolean;
  "aria-controls"?: string;
  "aria-haspopup"?: boolean | "menu" | "listbox" | "tree" | "grid" | "dialog";
  "aria-describedby"?: string;
  "data-testid"?: string;
  "data-lsp-state"?: string;
  "data-lsp-language"?: string;
  "data-state"?: string;
  "data-side"?: "top" | "right" | "bottom" | "left";
  "data-align"?: "start" | "center" | "end";
  "data-disabled"?: boolean | string;
  onClick?: React.MouseEventHandler<HTMLButtonElement>;
  onFocus?: React.FocusEventHandler<HTMLButtonElement>;
  onBlur?: React.FocusEventHandler<HTMLButtonElement>;
  onKeyDown?: React.KeyboardEventHandler<HTMLButtonElement>;
  onPointerDown?: React.PointerEventHandler<HTMLButtonElement>;
  onPointerUp?: React.PointerEventHandler<HTMLButtonElement>;
  onPointerCancel?: React.PointerEventHandler<HTMLButtonElement>;
  onPointerEnter?: React.PointerEventHandler<HTMLButtonElement>;
  onPointerMove?: React.PointerEventHandler<HTMLButtonElement>;
  onPointerLeave?: React.PointerEventHandler<HTMLButtonElement>;
  onLostPointerCapture?: React.PointerEventHandler<HTMLButtonElement>;
  onMouseEnter?: React.MouseEventHandler<HTMLButtonElement>;
  onMouseLeave?: React.MouseEventHandler<HTMLButtonElement>;
};

export function SurfaceAction(props: SurfaceActionProps) {
  const iconOnly = !props.text && !props.children;
  const hasIcon = props.icon !== undefined && props.icon !== null;

  return (
    <Button
      type="button"
      variant={props.surface === "topbar" ? "outline" : "ghost"}
      size={props.surface === "status-bar" ? "sm" : "default"}
      ref={props.ref}
      id={props.id}
      aria-label={props.label || undefined}
      title={props.title}
      aria-pressed={props.pressed}
      aria-busy={props.busy || undefined}
      aria-disabled={props.disabled || undefined}
      aria-expanded={props["aria-expanded"]}
      aria-controls={props["aria-controls"]}
      aria-haspopup={props["aria-haspopup"]}
      aria-describedby={props["aria-describedby"]}
      data-slot="surface-action"
      data-surface={props.surface}
      data-presentation={props.presentation}
      data-busy={props.busy || undefined}
      data-testid={props["data-testid"]}
      data-lsp-state={props["data-lsp-state"]}
      data-lsp-language={props["data-lsp-language"]}
      data-state={props["data-state"]}
      data-side={props["data-side"]}
      data-align={props["data-align"]}
      data-disabled={props["data-disabled"]}
      disabled={props.disabled}
      className={cn(
        "relative min-w-0 focus-visible:ring-[2px] aria-pressed:bg-muted",
        surfaceActionClassName(props.surface, props.presentation, iconOnly),
        TONE_CLASS[props.tone ?? "neutral"],
      )}
      onClick={props.onClick}
      onFocus={props.onFocus}
      onBlur={props.onBlur}
      onKeyDown={props.onKeyDown}
      onPointerDown={props.onPointerDown}
      onPointerUp={props.onPointerUp}
      onPointerCancel={props.onPointerCancel}
      onPointerEnter={props.onPointerEnter}
      onPointerMove={props.onPointerMove}
      onPointerLeave={props.onPointerLeave}
      onLostPointerCapture={props.onLostPointerCapture}
      onMouseEnter={props.onMouseEnter}
      onMouseLeave={props.onMouseLeave}
    >
      {hasIcon ? (
        <span
          aria-hidden="true"
          className={cn(
            "inline-flex shrink-0 items-center justify-center [&>*]:max-h-full [&>*]:max-w-full [&>svg]:!size-full",
            surfaceActionGlyphClassName(props.surface),
          )}
          data-slot="surface-action-icon"
        >
          {props.icon}
        </span>
      ) : null}
      {props.text ? (
        <span className="min-w-0 truncate" data-slot="surface-action-text">
          {props.text}
        </span>
      ) : null}
      {props.children}
      {props.badge ? (
        <span
          aria-hidden="true"
          className="max-w-8 truncate rounded-sm bg-muted px-1 text-[10px] leading-4 tabular-nums"
          data-slot="surface-action-badge"
        >
          {props.badge}
        </span>
      ) : null}
      {props.busy ? (
        <Spinner aria-hidden="true" className="absolute right-0.5 top-0.5 size-2" />
      ) : null}
    </Button>
  );
}
