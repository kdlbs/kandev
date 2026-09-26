import { describe, expect, it, vi } from "vitest";
import { createPluginComposerCapability } from "./composer-capability";

describe("createPluginComposerCapability", () => {
  it("fails closed after revocation", async () => {
    const insertText = vi.fn(() => true);
    const focus = vi.fn(() => true);
    const submit = vi.fn(async () => true);
    const capability = createPluginComposerCapability({ insertText, focus, submit });

    capability.revoke();

    expect(capability.api.insertText("hello")).toEqual({ status: "unavailable" });
    expect(capability.api.focus()).toEqual({ status: "unavailable" });
    await expect(capability.api.submit()).resolves.toEqual({ status: "unavailable" });
    expect(insertText).not.toHaveBeenCalled();
    expect(focus).not.toHaveBeenCalled();
    expect(submit).not.toHaveBeenCalled();
  });

  it("maps native operations to public outcomes", async () => {
    const capability = createPluginComposerCapability({
      insertText: () => true,
      focus: () => true,
      submit: async () => false,
    });

    expect(capability.api.insertText("hello")).toEqual({ status: "inserted" });
    expect(capability.api.focus()).toEqual({ status: "focused" });
    await expect(capability.api.submit()).resolves.toEqual({ status: "blocked" });
  });
});
