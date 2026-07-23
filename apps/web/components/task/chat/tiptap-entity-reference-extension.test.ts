import { describe, expect, it } from "vitest";
import * as entityReferenceExtensions from "./tiptap-entity-reference-extension";

describe("entityReference node", () => {
  it("provides a distinct inline atom for # references", () => {
    const extension = (entityReferenceExtensions as Record<string, unknown>).EntityReferenceNode as
      | { name?: string; config?: { inline?: boolean; atom?: boolean } }
      | undefined;

    expect(extension).toBeDefined();
    expect(extension?.name).toBe("entityReference");
    expect(extension?.config?.inline).toBe(true);
    expect(extension?.config?.atom).toBe(true);
  });

  it("renders the atom with a dedicated React node view", () => {
    const extension = (entityReferenceExtensions as Record<string, unknown>)
      .EntityReferenceNode as {
      config?: { addNodeView?: unknown };
    };

    expect(typeof extension.config?.addNodeView).toBe("function");
  });
});
