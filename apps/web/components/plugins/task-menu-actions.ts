import { createElement, isValidElement, type ReactNode } from "react";
import { pluginRegistry } from "@/lib/plugins/registry";
import { isPluginIconComponent, resolvePluginIcon } from "@/lib/plugins/icons";
import type { PluginTaskMenuActionRegistration } from "@/lib/plugins/registry-registration-types";
import type {
  PluginIcon,
  PluginTaskMenuContext,
  TaskMenuSubItemRegistration,
} from "@/lib/plugins/types";
import type { KanbanCardMenuEntry } from "../kanban-card-menu-items";

/**
 * Registered actions for `group`, filtered by `visible(context)` (default
 * visible when omitted). A `visible` that throws is caught, logged, and
 * treated as hidden — the same defensive handling as `run`'s rejection.
 */
export function visiblePluginMenuActions(
  group: PluginTaskMenuActionRegistration["group"],
  context: PluginTaskMenuContext,
): PluginTaskMenuActionRegistration[] {
  return pluginRegistry.getTaskMenuActions(group).filter((action) => {
    if (!action.visible) return true;
    try {
      return action.visible(context);
    } catch (error: unknown) {
      console.error(
        `[plugins] task menu action "${describeAction(action)}" visible() threw`,
        error,
      );
      return false;
    }
  });
}

/**
 * Plugin icons get the same sizing every neighbouring native menu icon uses.
 * A ready-made element — the shape registered before icon names/components
 * were resolved, and still shipped by kandev-plugin-tags — renders as-is:
 * passing it to the resolver would treat it as a name, miss the curated map,
 * and silently replace the plugin's own glyph with the fallback puzzle.
 */
function pluginMenuIcon(icon?: PluginIcon): ReactNode {
  if (!icon) return undefined;
  try {
    if (isValidElement(icon)) return icon;
    return createElement(resolvePluginIcon(icon), { className: "mr-2 h-4 w-4" });
  } catch {
    // `isValidElement` reads `$$typeof` again, so an icon whose accessors answer
    // once and then throw must mean "no icon" rather than a failed render.
    return undefined;
  }
}

/**
 * Plugin bundles are plain JavaScript, so the registration types are a
 * promise, not a guarantee. One log per action *and kind* is enough: `items()`
 * runs on every menu build, so a permanently malformed registration would
 * otherwise log on every render, while a later defect of a different kind on
 * the same action still reports. The set is never cleared, so the same kind
 * after re-registration stays silent for the life of the page.
 * The registry guards its own read of each registration before this module
 * receives a copy. The guards below handle malformed values in that copy.
 */
const loggedMenuDefects = new Set<string>();

/**
 * A plugin value's printable form. Template interpolation would call `ToString`,
 * which throws for a value that has no string form -- a `Symbol` id or a
 * null-prototype object -- and the omission paths are exactly where such a value
 * arrives, so the log must not be able to throw.
 */
function printable(value: unknown): string {
  if (typeof value === "string") return value;
  try {
    return String(value);
  } catch {
    // i18n-exempt: log placeholder for a value with no string form, never rendered.
    return "<unprintable>";
  }
}

/** The action's identity as a log label, read defensively for the same reason. */
function describeAction(action: PluginTaskMenuActionRegistration): string {
  try {
    return `${printable(action.pluginId)}:${printable(action.id)}`;
  } catch {
    // i18n-exempt: log placeholder for a registration that cannot be read, never rendered.
    return "<unreadable registration>";
  }
}

function logMenuDefect(
  action: PluginTaskMenuActionRegistration,
  kind: string,
  detail?: unknown,
): void {
  const actionLabel = describeAction(action);
  const key = `${actionLabel}:${kind}`;
  if (loggedMenuDefects.has(key)) return;
  loggedMenuDefects.add(key);
  console.error(`[plugins] task menu action "${actionLabel}" ${kind}`, detail);
}

/**
 * The action's own label, read once and only when it is a non-blank string.
 * A registration is plugin-authored data too, so a missing or hostile accessor
 * omits the entry (and reports it) rather than handing React an object child.
 */
/**
 * Escapes the three characters a menu key gives meaning to -- `%` first, so an
 * escape already inside an id cannot alias one this adds, then `:` and `#` -- and
 * leaves every other character verbatim.
 *
 * That is what makes the key injective for any plugin-authored string: every `%`
 * in the result starts one of exactly three two-character escapes, so decoding is
 * unambiguous, and neither the part separator (`:`) nor the child separator (`#`)
 * survives raw inside a part. A `%XX`-style per-code-unit escape cannot do this,
 * because a bare `%` separator is then indistinguishable from the start of an
 * escape: `p` + `abcd\uABCD` and `p\uABCD` + `abcd` would share one key.
 *
 * It also cannot throw, which is why it replaced `encodeURIComponent` here: that
 * raises `URIError` on a lone surrogate, and an exception on this path escapes a
 * card's render, where no error boundary catches it.
 */
function encodeKeyPart(value: string): string {
  return value.replace(/%/g, "%25").replace(/:/g, "%3A").replace(/#/g, "%23");
}

/** The action's own id, read once and only when it is a non-blank string. */
function readActionId(action: PluginTaskMenuActionRegistration): string | null {
  try {
    const { id } = action;
    return typeof id === "string" && id.trim().length > 0 ? id : null;
  } catch {
    return null;
  }
}

function readActionLabel(action: PluginTaskMenuActionRegistration): string | null {
  try {
    const { label } = action;
    return typeof label === "string" && label.trim().length > 0 ? label : null;
  } catch {
    return null;
  }
}

/** The action's own icon, read once; an unreadable one simply means no icon. */
function readActionIcon(action: PluginTaskMenuActionRegistration): ReactNode {
  try {
    return pluginMenuIcon(action.icon);
  } catch {
    logMenuDefect(action, "registration icon could not be read");
    return undefined;
  }
}

/**
 * An icon the menu can render: a curated name, a plugin-owned component
 * (including React's exotic components, which are objects at run time), or the
 * menu-only ready-made element. Everything else is a defect, while an absent
 * icon is null or undefined in a plain bundle.
 */
function isRenderableIcon(icon: unknown): boolean {
  if (icon === undefined || icon === null) return true;
  return typeof icon === "string" || isPluginIconComponent(icon) || isValidElement(icon);
}

/** One child, read once into the plain values the menu actually renders. */
type SubItemSnapshot = {
  id: string;
  label: string;
  icon?: PluginIcon;
  disabled?: boolean;
  run: (context: PluginTaskMenuContext) => void | Promise<void>;
};

/**
 * Reads one plugin-supplied child into a snapshot the render can trust: a
 * non-blank id and label, a callable run, and optional fields of the shapes
 * the entry builder understands (a curated name, a component, or a ready-made
 * element -- never an arbitrary object, which the icon resolver would coerce
 * into a key). Everything the render will read is read here, inside the
 * catch, so a throwing getter or a Proxy fails this child instead of the
 * card's render, and nothing is read twice.
 */
function readSubItem(item: unknown): SubItemSnapshot | null {
  if (!item || typeof item !== "object") return null;
  try {
    const { id, label, icon, disabled, run } = item as Partial<TaskMenuSubItemRegistration>;
    if (typeof id !== "string" || id.trim().length === 0) return null;
    if (typeof label !== "string" || label.trim().length === 0) return null;
    if (typeof run !== "function") return null;
    if (disabled !== undefined && disabled !== null && typeof disabled !== "boolean") return null;
    if (!isRenderableIcon(icon)) return null;
    return { id, label, icon: icon ?? undefined, disabled: disabled ?? undefined, run };
  } catch {
    return null;
  }
}

/**
 * The children of an action that declares `items`, or null when the action
 * has no usable submenu — leaving `run` as the entry's behavior.
 *
 * That fallback is the whole defensive boundary, because `items()` is a
 * plugin callback running inside the host's menu build, and every way its
 * result can be unreadable is the same answer: a throw, a thenable (the
 * contract is synchronous), a non-array, a result whose entries cannot be
 * rendered (blank/absent id or label, no callable run, an icon of the wrong
 * shape), a child that cannot even be read (a throwing getter, a Proxy), and
 * a repeated id all degrade to the flat item — or drop just that child —
 * instead of crashing the card's render, producing a trigger nothing can
 * open, or handing React children it cannot key. The body is one `try` on
 * purpose: a plugin object is free to throw from any property access,
 * including the array methods and the `then` lookup this function would
 * otherwise perform on it.
 */
function pluginSubItems(
  action: PluginTaskMenuActionRegistration,
  context: PluginTaskMenuContext,
): readonly SubItemSnapshot[] | null {
  try {
    if (typeof action.items !== "function") return null;
    const result: unknown = action.items(context);

    if (result && typeof (result as { then?: unknown }).then === "function") {
      logMenuDefect(action, "items() returned a promise; it must be synchronous");
      // Observe the rejection so an out-of-contract async callback cannot
      // surface as an unhandled rejection out of the host's menu build.
      void Promise.resolve(result).catch((error: unknown) => {
        logMenuDefect(action, "items() rejected", error);
      });
      return null;
    }

    if (!Array.isArray(result)) {
      // null/undefined means "no children"; anything else is out of contract.
      if (result !== null && result !== undefined) {
        logMenuDefect(action, "items() must return an array");
      }
      return null;
    }

    // Array.from copies through the array's own length and indices, so no
    // array method is looked up on the plugin's object.
    const list = Array.from(result as readonly unknown[]);
    const seen = new Set<string>();
    const usable: SubItemSnapshot[] = [];
    list.forEach((item) => {
      const child = readSubItem(item);
      if (!child) {
        logMenuDefect(action, "items() returned an unusable child", item);
        return;
      }
      if (seen.has(child.id)) {
        logMenuDefect(action, "items() repeated a child id", child.id);
        return;
      }
      seen.add(child.id);
      usable.push(child);
    });
    return usable.length > 0 ? usable : null;
  } catch (error: unknown) {
    logMenuDefect(action, "items() could not be read", error);
    return null;
  }
}

/**
 * `Promise.resolve().then(() => run(context))` — not
 * `Promise.resolve(run(context))` — so a *synchronous* throw inside run()
 * also lands in the `.catch()` below. Calling run directly as the
 * Promise.resolve() argument still throws before that expression finishes
 * evaluating, escaping past .catch() entirely and straight out of this
 * onSelect handler. `subject` names the failing part of a submenu action.
 */
function runPluginCallback(
  action: PluginTaskMenuActionRegistration,
  run: (context: PluginTaskMenuContext) => void | Promise<void>,
  context: PluginTaskMenuContext,
  subject = "",
): void {
  Promise.resolve()
    .then(() => run(context))
    .catch((error: unknown) => {
      console.error(
        `[plugins] task menu action "${action.pluginId}:${action.id}"${subject} failed`,
        error,
      );
    });
}

/**
 * Builds the menu entry for one plugin task menu action: a submenu when the
 * action declares a non-empty `items(context)`, otherwise the flat item it
 * has always been (and still is on a host that predates submenus, which
 * ignores `items` and reads only `label`/`icon`/`run`).
 */
export function pluginMenuEntry(
  action: PluginTaskMenuActionRegistration,
  context: PluginTaskMenuContext,
  disabled?: boolean,
  keyPrefix = "plugin-edit",
): KanbanCardMenuEntry | null {
  const label = readActionLabel(action);
  if (label === null) {
    logMenuDefect(action, "registration has no usable label");
    return null;
  }
  const actionId = readActionId(action);
  if (actionId === null) {
    logMenuDefect(action, "registration has no usable id");
    return null;
  }
  const icon = readActionIcon(action);
  // Every plugin-controlled part is percent-encoded, so a dash-join cannot spell
  // another action's key (`p` + `q-x` vs `p-q` + `x`) and no part can contain the
  // separators: the palette flattens every plugin entry into one list where the
  // key is both a React key and cmdk's value.
  const key = `${keyPrefix}-${encodeKeyPart(action.pluginId)}:${encodeKeyPart(actionId)}`;
  const items = pluginSubItems(action, context);

  if (items) {
    return {
      kind: "submenu",
      key,
      icon,
      label,
      disabled,
      children: items.map((item) => ({
        kind: "item",
        // The id is plugin-controlled and the action key is itself a dash-join
        // of plugin-controlled ids, so `-${id}` can spell another action's child
        // key exactly; the palette flattens every plugin entry into one list and
        // uses this key as both its React key and cmdk's value. The separator
        // stays unambiguous because an id's own `#` is percent-encoded.
        key: `${key}#${encodeKeyPart(item.id)}`,
        icon: pluginMenuIcon(item.icon),
        label: item.label,
        disabled: disabled || item.disabled,
        onSelect: () => runPluginCallback(action, item.run, context, ` item "${item.id}"`),
      })),
    };
  }

  if (typeof action.run !== "function") {
    logMenuDefect(action, "registration has no run");
    return null;
  }
  return {
    kind: "item",
    key,
    icon,
    label,
    disabled,
    onSelect: () => runPluginCallback(action, action.run, context),
  };
}

/**
 * Builds flat, top-level menu entries for group "primary" task menu
 * actions, in registration order. Unlike group "edit" (nested in the Edit
 * submenu), these render directly in the card menu — after the movement
 * group and before the Archive/Delete removal group. An action that declares
 * `items` is the one exception: it renders as its own submenu.
 */
export function buildPrimaryPluginEntries({
  disabled,
  context,
}: {
  disabled?: boolean;
  context: PluginTaskMenuContext;
}): KanbanCardMenuEntry[] {
  return visiblePluginMenuActions("primary", context)
    .map((action) => pluginMenuEntry(action, context, disabled, "plugin-primary"))
    .filter((entry): entry is KanbanCardMenuEntry => entry !== null);
}
