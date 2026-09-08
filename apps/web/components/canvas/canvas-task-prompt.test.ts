import enCanvases from "@/src/locales/en/canvases.json";
import pseudoCanvases from "@/src/locales/pseudo/canvases.json";
import ptCanvases from "@/src/locales/pt-pt/canvases.json";
import zhCnCanvases from "@/src/locales/zh-cn/canvases.json";
import zhHkCanvases from "@/src/locales/zh-hk/canvases.json";
import zhTwCanvases from "@/src/locales/zh-tw/canvases.json";
import { describe, expect, it } from "vitest";

const canvasCatalogs = {
  en: enCanvases,
  "pt-pt": ptCanvases,
  "zh-cn": zhCnCanvases,
  "zh-hk": zhHkCanvases,
  "zh-tw": zhTwCanvases,
  pseudo: pseudoCanvases,
};

const requiredCanvasTools = [
  "create_canvas_kandev",
  "read_canvas_authoring_skill_kandev",
  "publish_canvas_kandev",
];

describe("canvas creation task preset", () => {
  it("keeps the authoring workflow and callable tool names in every catalog", () => {
    for (const [locale, catalog] of Object.entries(canvasCatalogs)) {
      const prompt = catalog.createCanvasTaskPrompt;
      expect(prompt, locale).toMatch(/\S/);
      for (const tool of requiredCanvasTools) {
        expect(prompt, `${locale} is missing ${tool}`).toContain(tool);
      }
    }
  });

  it("requires Kandev publication instead of treating files or a build as a release", () => {
    const prompt = enCanvases.createCanvasTaskPrompt;

    expect(prompt).toContain("Create the draft in Kandev");
    expect(prompt).toContain("Build inside the returned directory");
    expect(prompt).toContain("A local build alone does not publish a canvas inside Kandev");
    expect(prompt).toContain("report the limitation instead of claiming");
  });
});
