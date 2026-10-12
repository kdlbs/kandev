import enCanvases from "@/src/locales/en/canvases.json";
import pseudoCanvases from "@/src/locales/pseudo/canvases.json";
import ptCanvases from "@/src/locales/pt-pt/canvases.json";
import zhCnCanvases from "@/src/locales/zh-cn/canvases.json";
import zhHkCanvases from "@/src/locales/zh-hk/canvases.json";
import zhTwCanvases from "@/src/locales/zh-tw/canvases.json";
import { buildCanvasCreateTaskPrompt, CANVAS_CREATE_PROMPT_REFERENCE } from "./canvas-task-prompt";
import { describe, expect, it } from "vitest";

const canvasCatalogs = {
  en: enCanvases,
  "pt-pt": ptCanvases,
  "zh-cn": zhCnCanvases,
  "zh-hk": zhHkCanvases,
  "zh-tw": zhTwCanvases,
  pseudo: pseudoCanvases,
};

describe("canvas creation task preset", () => {
  it("keeps a short localized goal in every catalog", () => {
    for (const [locale, catalog] of Object.entries(canvasCatalogs)) {
      const prompt = catalog.createCanvasTaskPrompt;
      expect(prompt, locale).toMatch(/\S/);
      expect(prompt, locale).not.toContain("create_canvas_kandev");
      expect(prompt, locale).not.toContain("read_canvas_authoring_skill_kandev");
      expect(prompt, locale).not.toContain("publish_canvas_kandev");
    }
  });

  it("appends the exact saved-prompt reference outside localization", () => {
    for (const [locale, catalog] of Object.entries(canvasCatalogs)) {
      expect(buildCanvasCreateTaskPrompt(catalog.createCanvasTaskPrompt), locale).toBe(
        `${catalog.createCanvasTaskPrompt}\n\n${CANVAS_CREATE_PROMPT_REFERENCE}`,
      );
    }
  });

  it("keeps the English goal editable while leaving authoring instructions to the saved prompt", () => {
    expect(enCanvases.createCanvasTaskPrompt).toBe(
      "Create a new Kandev canvas with a coordinator view that lists the existing tasks.",
    );
    expect(buildCanvasCreateTaskPrompt(enCanvases.createCanvasTaskPrompt)).toBe(
      "Create a new Kandev canvas with a coordinator view that lists the existing tasks.\n\n@create-canvas",
    );
  });
});
