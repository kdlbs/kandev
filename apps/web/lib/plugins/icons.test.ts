import { createElement, forwardRef, lazy } from "react";
import { describe, expect, it } from "vitest";
import { IconPuzzle, IconTicket } from "@tabler/icons-react";
import { isPluginIconComponent, lookupPluginIcon, resolvePluginIcon } from "./icons";

describe("plugin icons", () => {
  it("looks up a known icon name", () => {
    expect(lookupPluginIcon("ticket")).toBe(IconTicket);
    expect(resolvePluginIcon("ticket")).toBe(IconTicket);
  });

  it("does not require host-owned provider brand icons", () => {
    expect(lookupPluginIcon("bitbucket")).toBeUndefined();
    expect(resolvePluginIcon("bitbucket")).toBe(IconPuzzle);
  });

  it("passes through a plugin-owned icon component", () => {
    const PluginIcon = () => null;

    expect(lookupPluginIcon(PluginIcon as unknown as string)).toBe(PluginIcon);
    expect(resolvePluginIcon(PluginIcon as unknown as string)).toBe(PluginIcon);
  });

  it("returns undefined from lookupPluginIcon for unknown or missing names", () => {
    expect(lookupPluginIcon("not-an-icon")).toBeUndefined();
    expect(lookupPluginIcon(undefined)).toBeUndefined();
  });

  // Regression: a JavaScript bundle can pass any value. Indexing PLUGIN_ICONS
  // with a non-string coerced it to a key, so an object whose toString threw
  // took the rendering surface down, and any other object silently became the
  // puzzle glyph by way of a "[object Object]" lookup miss.
  it("passes through an exotic component, which memo/forwardRef produce", () => {
    // React's memo and forwardRef return objects that are callable per their
    // types but not at run time, and @tabler/icons-react -- the set
    // PLUGIN_ICONS maps names onto -- builds every icon with forwardRef.
    // memo() returns a non-callable object; a function carrying $$typeof would
    // not reproduce the shape that `typeof icon === "function"` accepts.
    const ExoticIcon = { $$typeof: Symbol.for("react.memo"), type: () => null };

    expect(lookupPluginIcon(ExoticIcon as never)).toBe(ExoticIcon);
    expect(resolvePluginIcon(ExoticIcon as never)).toBe(ExoticIcon);
  });

  it("does not treat an element, a lazy component or a bare tag as a component", () => {
    // An element is a rendered node, so handing it to createElement on the twelve
    // surfaces that do `createElement(resolvePluginIcon(icon))` would throw; a
    // lazy component cannot resolve synchronously in a menu; and an object that
    // merely carries `$$typeof` is not a component at all. All three fall back.
    const element = createElement("svg", { viewBox: "0 0 24 24" });
    const lazyIcon = lazy(() => Promise.resolve({ default: () => null }));
    const bogus = { $$typeof: 1 };

    for (const icon of [element, lazyIcon, bogus]) {
      expect(isPluginIconComponent(icon)).toBe(false);
      expect(lookupPluginIcon(icon as never)).toBeUndefined();
      expect(resolvePluginIcon(icon as never)).toBe(IconPuzzle);
    }
  });

  it("passes through a forwardRef component, the shape an icon set builds", () => {
    const Forwarded = forwardRef<SVGSVGElement, { className?: string }>(() => null);

    expect(isPluginIconComponent(Forwarded)).toBe(true);
    expect(lookupPluginIcon(Forwarded as never)).toBe(Forwarded);
  });

  it("treats an icon whose tag read throws as absent, not as a component", () => {
    // Regression: the tag read (and the `in` check before it) runs a
    // plugin-supplied trap, and this function backs every icon surface.
    const hostile = new Proxy(
      {},
      {
        get() {
          throw new Error("hostile $$typeof getter");
        },
        has() {
          throw new Error("hostile in trap");
        },
      },
    );

    expect(() => isPluginIconComponent(hostile)).not.toThrow();
    expect(isPluginIconComponent(hostile)).toBe(false);
    expect(lookupPluginIcon(hostile as never)).toBeUndefined();
    expect(resolvePluginIcon(hostile as never)).toBe(IconPuzzle);
  });

  it("resolves only own keys, so a prototype member is not an icon", () => {
    for (const name of ["__proto__", "constructor", "toString", "hasOwnProperty"]) {
      expect(lookupPluginIcon(name)).toBeUndefined();
      expect(resolvePluginIcon(name)).toBe(IconPuzzle);
    }
    expect(lookupPluginIcon("ticket")).toBe(IconTicket);
  });

  it("treats a non-string, non-function icon as absent instead of coercing it to a name", () => {
    const hostile = {
      toString() {
        throw new Error("key coercion");
      },
    };

    expect(() => lookupPluginIcon(hostile as unknown as string)).not.toThrow();
    expect(lookupPluginIcon(hostile as unknown as string)).toBeUndefined();
    expect(resolvePluginIcon(hostile as unknown as string)).toBe(IconPuzzle);
    expect(lookupPluginIcon({} as unknown as string)).toBeUndefined();
  });

  it("falls back to the puzzle glyph from resolvePluginIcon", () => {
    expect(resolvePluginIcon("not-an-icon")).toBe(IconPuzzle);
    expect(resolvePluginIcon(undefined)).toBe(IconPuzzle);
  });
});
