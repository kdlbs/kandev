import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { pluginRegistry } from "@/lib/plugins/registry";
import type {
  PluginIcon,
  PluginTaskMenuContext,
  TaskMenuSubItemRegistration,
} from "@/lib/plugins/types";
import {
  buildCardPluginEntries,
  KanbanCardDropdownMenuItems,
  type KanbanCardMenuEntry,
} from "../kanban-card-menu-items";
import { buildPrimaryPluginEntries } from "./task-menu-actions";

const PLUGIN_ID = "kandev-plugin-tags";
const ACTION_ID = "add-tag";
const ACTION_LABEL = "Add tag";
const ACTION_KEY = `plugin-primary-${PLUGIN_ID}:${ACTION_ID}`;

const CONTEXT: PluginTaskMenuContext = {
  workspaceId: "ws-1",
  taskId: "task-1",
  taskTitle: "Fix the bug",
  workflowStepId: "step-1",
  presentation: "desktop",
};

function registerAction(
  overrides: {
    id?: string;
    label?: string;
    group?: "edit" | "primary";
    icon?: PluginIcon;
    run?: (context: PluginTaskMenuContext) => void | Promise<void>;
    // Deliberately wider than the contract: this helper also registers results
    // the types forbid (a promise, an unknown-returning wrapper), which is what
    // a JavaScript bundle can actually send.
    items?: (context: PluginTaskMenuContext) => unknown;
  } = {},
) {
  const run = overrides.run ?? vi.fn();
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: overrides.id ?? ACTION_ID,
    label: overrides.label ?? ACTION_LABEL,
    group: overrides.group ?? "primary",
    run,
    ...(overrides.icon ? { icon: overrides.icon } : {}),
    ...(overrides.items
      ? {
          items: overrides.items as (
            context: PluginTaskMenuContext,
          ) => readonly TaskMenuSubItemRegistration[],
        }
      : {}),
  });
  return run;
}

function renderEntries(entries: KanbanCardMenuEntry[]) {
  render(
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger>open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <KanbanCardDropdownMenuItems entries={entries} />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
}

/** Radix opens a submenu on a mouse pointer-move over its trigger. */
async function openSubmenu(label: string, childLabel: string) {
  fireEvent.pointerMove(screen.getByRole("menuitem", { name: label }), { pointerType: "mouse" });
  return screen.findByRole("menuitem", { name: childLabel });
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("buildPrimaryPluginEntries — action without items", () => {
  it("stays a flat item that runs run(context)", async () => {
    const run = registerAction();
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    fireEvent.click(screen.getByRole("menuitem", { name: ACTION_LABEL }));

    await Promise.resolve();
    expect(run).toHaveBeenCalledWith(CONTEXT);
  });
});

describe("buildPrimaryPluginEntries — action declaring items", () => {
  it("renders a submenu whose children are the resolved items, in order", () => {
    registerAction({
      items: () => [
        { id: "more", label: "More tags", run: vi.fn() },
        { id: "blocked", label: "Blocked", run: vi.fn() },
      ],
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries.map((entry) => entry.kind)).toEqual(["submenu"]);
    const entry = entries[0];
    if (entry.kind !== "submenu") return;
    expect(entry.key).toBe(ACTION_KEY);
    expect(entry.label).toBe(ACTION_LABEL);
    expect(entry.children.map((child) => child.key)).toEqual([
      `${ACTION_KEY}#more`,
      `${ACTION_KEY}#blocked`,
    ]);
    // Every child is a selectable item, not a separator or a nested submenu.
    expect(
      entry.children.map((child) => (child.kind === "item" ? child.label : child.kind)),
    ).toEqual(["More tags", "Blocked"]);
  });

  it("runs the selected child with the menu context, not the action's fallback", async () => {
    const more = vi.fn();
    const blocked = vi.fn();
    const run = registerAction({
      items: () => [
        { id: "more", label: "More tags", run: more },
        { id: "blocked", label: "Blocked", run: blocked },
      ],
    });
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    const blockedItem = await openSubmenu(ACTION_LABEL, "Blocked");
    fireEvent.click(blockedItem);

    await Promise.resolve();
    expect(blocked).toHaveBeenCalledWith(CONTEXT);
    expect(more).not.toHaveBeenCalled();
    expect(run).not.toHaveBeenCalled();
  });

  it("keeps a disabled child unselectable", async () => {
    const disabledRun = vi.fn();
    registerAction({
      items: () => [{ id: "blocked", label: "Blocked", disabled: true, run: disabledRun }],
    });
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    const child = await openSubmenu(ACTION_LABEL, "Blocked");
    fireEvent.click(child);

    await Promise.resolve();
    expect(disabledRun).not.toHaveBeenCalled();
  });

  it("disables every child while the entry itself is disabled", async () => {
    const childRun = vi.fn();
    registerAction({ items: () => [{ id: "blocked", label: "Blocked", run: childRun }] });

    const entry = buildPrimaryPluginEntries({ context: CONTEXT, disabled: true })[0];
    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;
    // A disabled trigger cannot be opened at all; rendering its children
    // directly is what proves the disabled flag reached them.
    expect(entry.disabled).toBe(true);
    renderEntries(entry.children);

    fireEvent.click(screen.getByRole("menuitem", { name: "Blocked" }));

    await Promise.resolve();
    expect(childRun).not.toHaveBeenCalled();
  });

  // A plugin's items() runs inside the host's menu build, so an unusable list
  // must never produce a trigger with nothing behind it: the action's run() is
  // what a host predating `items` would have used anyway.
  it("logs and renders the flat fallback when items() throws", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({
      items: () => {
        throw new Error("items() blew up");
      },
    });

    expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
      "item",
    ]);
    expect(consoleErrorSpy).toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });

  it("renders the flat fallback when items() yields nothing", () => {
    registerAction({ items: () => [] });

    expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
      "item",
    ]);
  });

  it("logs a rejecting child run without throwing", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({
      items: () => [
        { id: "blocked", label: "Blocked", run: () => Promise.reject(new Error("boom")) },
      ],
    });
    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    const child = await openSubmenu(ACTION_LABEL, "Blocked");
    expect(() => fireEvent.click(child)).not.toThrow();

    await vi.waitFor(() => expect(consoleErrorSpy).toHaveBeenCalled());
    consoleErrorSpy.mockRestore();
  });
});

describe("buildPrimaryPluginEntries — malformed plugin results", () => {
  // Plugin bundles are plain JavaScript: the registration types are a promise,
  // not a guarantee. Every shape a broken plugin can produce must degrade to
  // the flat fallback (or drop just the unusable child) instead of crashing the
  // card render or handing React children it cannot key. Each case registers a
  // distinct action id, since one broken action is reported once -- the host
  // rebuilds entries on every render, so a per-build log would spam.
  const fallbackCases: [string, () => unknown][] = [
    ["a null child", () => [null]],
    ["a child without an id or label", () => [{ run: vi.fn() }]],
    ["a child with a blank id", () => [{ id: "   ", label: "X", run: vi.fn() }]],
    ["a child with a blank label", () => [{ id: "x", label: "  ", run: vi.fn() }]],
    [
      "a child with a non-boolean disabled",
      () => [{ id: "x", label: "X", run: vi.fn(), disabled: 1 }],
    ],
    ["a child without a run", () => [{ id: "x", label: "X" }]],
    ["a child with an unusable icon", () => [{ id: "x", label: "X", icon: {}, run: vi.fn() }]],
    ["a non-array result", () => "nope"],
    ["a promise", () => Promise.resolve([{ id: "x", label: "X", run: vi.fn() }])],
    [
      "an array whose methods were replaced",
      () =>
        new Proxy([{ id: "x", label: "X", run: vi.fn() }], {
          get() {
            throw new Error("trap");
          },
        }),
    ],
    [
      "a thenable that throws on lookup",
      () =>
        Object.defineProperty({}, "then", {
          get() {
            throw new Error("then getter");
          },
        }),
    ],
  ];

  for (const [index, [label, items]] of fallbackCases.entries()) {
    it(`falls back to the flat item and reports ${label} once`, async () => {
      const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      registerAction({ id: `broken-${index}`, items });

      expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
        "item",
      ]);
      // A second build must not log again: the defect is permanent for a
      // loaded bundle, and this callback runs on every render.
      expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
        "item",
      ]);
      expect(consoleErrorSpy).toHaveBeenCalledTimes(1);

      consoleErrorSpy.mockRestore();
      // A rejected promise must not escape as an unhandled rejection; one
      // macrotask is enough for vitest to surface it if it were unobserved.
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }

  it("observes a rejected promise, so it cannot escape as an unhandled rejection", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({ id: "rejecting", items: () => Promise.reject(new Error("boom")) });

    expect(buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry.kind)).toEqual([
      "item",
    ]);
    // Vitest fails the file on an unhandled rejection, so reaching the end of
    // this test at all proves the rejection was observed -- but the log has to
    // say so too, which the deleted-observation mutation otherwise hides.
    await vi.waitFor(() =>
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining("items() rejected"),
        expect.any(Error),
      ),
    );
    consoleErrorSpy.mockRestore();
  });
});

describe("buildPrimaryPluginEntries — unreadable and repeated children", () => {
  it("reports a repeated child id and keeps the first entry", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const first = { id: "dup", label: "First", run: vi.fn() };
    registerAction({
      id: "duplicated",
      items: () => [first, { id: "dup", label: "Second", run: vi.fn() }],
    });

    const entry = buildPrimaryPluginEntries({ context: CONTEXT })[0];

    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      `plugin-primary-${PLUGIN_ID}:duplicated#dup`,
    ]);
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining("repeated a child id"),
      "dup",
    );
    consoleErrorSpy.mockRestore();
  });

  it("reports a later defect of a different kind on the same action", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    let impl: () => unknown = () => [null];
    registerAction({ id: "varies", items: () => impl() });

    buildPrimaryPluginEntries({ context: CONTEXT });
    impl = () => {
      throw new Error("now it throws");
    };
    buildPrimaryPluginEntries({ context: CONTEXT });

    expect(consoleErrorSpy.mock.calls.map((call) => String(call[0]))).toEqual([
      expect.stringContaining("unusable child"),
      expect.stringContaining("could not be read"),
    ]);
    consoleErrorSpy.mockRestore();
  });

  it("falls back when a child cannot be read at all", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const hostile = { id: "x", label: "X", run: vi.fn() };
    Object.defineProperty(hostile, "icon", {
      enumerable: true,
      get() {
        throw new Error("getter blew up");
      },
    });
    registerAction({ id: "hostile-child", items: () => [hostile] });

    let entries: KanbanCardMenuEntry[] = [];
    expect(() => {
      entries = buildPrimaryPluginEntries({ context: CONTEXT });
    }).not.toThrow();
    expect(entries.map((entry) => entry.kind)).toEqual(["item"]);
    expect(consoleErrorSpy).toHaveBeenCalledTimes(1);
    consoleErrorSpy.mockRestore();
  });

  it("keeps the usable children and drops only the broken ones", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({
      id: "partly-broken",
      items: () => [
        { id: "gone", label: "Gone", run: vi.fn() },
        { id: "", label: "No id", run: vi.fn() },
        { id: "ok", label: "Ok", run: vi.fn() },
        { id: "gone", label: "Duplicate id", run: vi.fn() },
      ],
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries.map((entry) => entry.kind)).toEqual(["submenu"]);
    const entry = entries[0];
    if (entry.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      `plugin-primary-${PLUGIN_ID}:partly-broken#gone`,
      `plugin-primary-${PLUGIN_ID}:partly-broken#ok`,
    ]);
    consoleErrorSpy.mockRestore();
  });
});

describe("buildPrimaryPluginEntries — unusable registrations and array-likes", () => {
  // A registration is plugin-authored data as much as its `items()` result: an
  // action whose own fields cannot be read degrades to no entry instead of
  // handing React an object child, and a result whose array methods are
  // hostile must be copied rather than called.
  it("omits an action whose label is not a usable string, and logs once", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    // Registered around the helper, which defaults a missing label.
    const broken = (id: string, label: unknown) => ({ id, label, group: "primary", run: vi.fn() });
    pluginRegistry
      .forPlugin(PLUGIN_ID)
      .registerTaskMenuAction(broken("blank-label", "   ") as never);
    pluginRegistry
      .forPlugin(PLUGIN_ID)
      .registerTaskMenuAction(broken("object-label", { toString: () => "not a label" }) as never);

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries).toEqual([]);
    // One report per action: the two registrations fail the same way but are
    // different action ids.
    expect(consoleErrorSpy).toHaveBeenCalledTimes(2);
    consoleErrorSpy.mockRestore();
  });

  it("renders an action whose icon is an object that refuses to be coerced", () => {
    // The icon resolver looks a name up in its curated map, so a non-string
    // icon is no name at all: the entry keeps rendering with the fallback glyph
    // instead of throwing out of the card's render while the map coerces the
    // object to a key.
    const icon = {
      toString() {
        throw new Error("key coercion");
      },
    };
    registerAction({
      id: "object-icon",
      label: "Object icon",
      icon: icon as unknown as PluginIcon,
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries.map((entry) => entry.kind)).toEqual(["item"]);
    const entry = entries[0];
    if (entry.kind !== "item") return;
    expect(entry.label).toBe("Object icon");
    expect(entry.icon).toBeDefined();
  });

  it("copies a hostile array-like instead of calling its methods", () => {
    class HostileArray extends Array<unknown> {
      forEach(): void {
        throw new Error("forEach is not yours to call");
      }
      map(): never {
        throw new Error("map is not yours to call");
      }
    }
    const items = HostileArray.from([{ id: "child", label: "Child", run: vi.fn() }]);
    registerAction({ id: "hostile-array", items: () => items });

    const entry = buildPrimaryPluginEntries({ context: CONTEXT })[0];

    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      `plugin-primary-${PLUGIN_ID}:hostile-array#child`,
    ]);
  });

  it("treats every processing flag as disabling the plugin entries, on both paths", () => {
    registerAction({ id: "flagged", items: () => [{ id: "child", label: "Child", run: vi.fn() }] });

    for (const flag of ["disabled", "isDeleting", "isArchiving", "isDetaching"] as const) {
      const inputs = { pluginMenuContext: CONTEXT, [flag]: true };
      const prebuilt = buildCardPluginEntries(inputs).primary.find(
        (entry) => entry.key === `plugin-primary-${PLUGIN_ID}:flagged`,
      );
      const internal = buildPrimaryPluginEntries({ context: CONTEXT, disabled: true })[0];
      // Both paths must agree, and a submenu's children inherit the state.
      const prebuiltSubmenu = prebuilt?.kind === "submenu" ? prebuilt : undefined;
      const internalSubmenu = internal.kind === "submenu" ? internal : undefined;
      expect(prebuiltSubmenu?.disabled, flag).toBe(true);
      expect(internalSubmenu?.disabled, flag).toBe(true);
      expect(
        prebuiltSubmenu?.children.map((child) =>
          child.kind === "item" ? child.disabled : undefined,
        ),
        flag,
      ).toEqual([true]);
    }
  });
});

describe("buildPrimaryPluginEntries — shapes that must survive the boundary", () => {
  it("keeps a child whose icon is an exotic component", () => {
    // React's memo/forwardRef/lazy return objects, not functions, and an icon
    // set -- which host-api tells plugins to bundle -- is built from
    // forwardRef, so rejecting objects would drop a contract-valid child.
    // memo() returns a non-callable object; a function carrying $$typeof would
    // not reproduce the shape that `typeof icon === "function"` accepts.
    const ExoticIcon = { $$typeof: Symbol.for("react.memo"), type: () => null };
    registerAction({
      id: "exotic-icon",
      items: () => [{ id: "ok", label: "Ok", icon: ExoticIcon as never, run: vi.fn() }],
    });

    const entry = buildPrimaryPluginEntries({ context: CONTEXT })[0];
    expect(entry?.kind).toBe("submenu");
    if (entry?.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      `plugin-primary-${PLUGIN_ID}:exotic-icon#ok`,
    ]);
  });

  it("keeps a child with an absent or null icon and drops an unusable one", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({
      id: "icon-shapes",
      items: () => [
        { id: "null-icon", label: "Null icon", icon: null, run: vi.fn() },
        { id: "no-icon", label: "No icon", run: vi.fn() },
        { id: "array-icon", label: "Array icon", icon: [] as never, run: vi.fn() },
        { id: "object-icon", label: "Object icon", icon: {} as never, run: vi.fn() },
        { id: "null-disabled", label: "Null disabled", disabled: null as never, run: vi.fn() },
      ],
    });

    const entry = buildPrimaryPluginEntries({ context: CONTEXT })[0];
    expect(entry?.kind).toBe("submenu");
    if (entry?.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      `plugin-primary-${PLUGIN_ID}:icon-shapes#null-icon`,
      `plugin-primary-${PLUGIN_ID}:icon-shapes#no-icon`,
      `plugin-primary-${PLUGIN_ID}:icon-shapes#null-disabled`,
    ]);
    // The two unusable shapes are one defect of the same kind: one report.
    expect(consoleErrorSpy).toHaveBeenCalledTimes(1);
    consoleErrorSpy.mockRestore();
  });
});

describe("buildPrimaryPluginEntries — ids the encoder must survive", () => {
  // Regression: `encodeURIComponent` raises `URIError` on a lone surrogate, which
  // a truncated astral character (`slice`, `[0]`) produces. The throw left
  // `pluginMenuEntry`, so it escaped the builder called during a card's render and
  // unmounted the whole route -- and a lone surrogate is a non-blank string, so the
  // id guard did not catch it either.
  it("builds an entry whose id is a lone surrogate instead of throwing", () => {
    const loneSurrogate = "\ud83c";
    registerAction({
      id: loneSurrogate,
      items: () => [{ id: loneSurrogate, label: "Child", run: vi.fn() }],
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries).toHaveLength(1);
    expect(entries[0]?.key).toContain(loneSurrogate);
    expect(entries[0]?.kind === "submenu" ? entries[0].children[0]?.key : "").toContain(
      loneSurrogate,
    );
  });

  it("keeps an escaped unit from running into the literal after it", () => {
    // Regression from review: an encoder that emits `%XX`-style tokens cannot use a
    // bare `%` as the part separator, because the separator is then indistinguishable
    // from the start of a token. Only `%`, `:` and `#` are escaped now.
    registerAction({ id: "%0", label: "Percent" });
    registerAction({ id: "\u0250", label: "Unit" });
    // The same class one width up: a two-digit escape plus one literal hex digit.
    registerAction({ id: "\u00e9a", label: "Accented" });
    registerAction({ id: "\u0e9a", label: "Thai" });
    registerAction({
      id: "children",
      label: "Children",
      items: () => [
        { id: "\u00e9a", label: "Accented child", run: vi.fn() },
        { id: "\u0e9a", label: "Thai child", run: vi.fn() },
      ],
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });
    const keys = entries.flatMap((entry) =>
      entry
        ? [entry.key, ...(entry.kind === "submenu" ? entry.children.map((c) => c.key) : [])]
        : [],
    );

    expect(keys).toHaveLength(7);
    expect(new Set(keys).size).toBe(7);
  });

  it("keeps a lone-surrogate id distinct from a valid astral one", () => {
    registerAction({ id: "\ud83c", label: "Half" });
    registerAction({ id: "\ud83c\udf89", label: "Whole" });

    const keys = buildPrimaryPluginEntries({ context: CONTEXT }).map((entry) => entry?.key);

    expect(new Set(keys).size).toBe(2);
  });
});

describe("buildPrimaryPluginEntries — mixed registrations", () => {
  it("renders a submenu action as one entry and keeps the rest flat", () => {
    registerAction({ id: "add-tag", items: () => [{ id: "more", label: "More", run: vi.fn() }] });
    registerAction({ id: "other", label: "Other action" });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });

    expect(entries.map((entry) => entry.kind)).toEqual(["submenu", "item"]);
    expect(entries.map((entry) => entry.key)).toEqual([
      ACTION_KEY,
      `plugin-primary-${PLUGIN_ID}:other`,
    ]);
  });
});

describe("buildPrimaryPluginEntries — icon resolution", () => {
  // Regression: icons registered before the name/component resolution landed
  // are ready-made elements (kandev-plugin-tags ships one). Reading one as a
  // name looked up `PLUGIN_ICONS["[object Object]"]` and rendered the puzzle
  // fallback glyph instead of the plugin's own icon.
  it("renders an element-form icon instead of the fallback glyph", () => {
    const elementIcon = createElement(
      "svg",
      { "data-testid": "plugin-menu-icon", viewBox: "0 0 24 24" },
      createElement("path", { d: "M4 4h16" }),
    );
    registerAction({ icon: elementIcon as unknown as PluginIcon });

    renderEntries(buildPrimaryPluginEntries({ context: CONTEXT }));

    expect(screen.getByTestId("plugin-menu-icon")).toBeTruthy();
  });
});
