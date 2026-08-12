import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  convertMessage,
  loadGlossary,
  protectLiterals,
} from "./lib/zh-hant-convert.mjs";

loadGlossary();

describe("protectLiterals", () => {
  it("restores interpolation and Trans tags after conversion", () => {
    const source = "刪除 <1>{{count}}</1> 项";
    const { text, restore } = protectLiterals(source);
    assert.equal(text.includes("{{count}}"), false);
    assert.equal(text.includes("<1>"), false);
    assert.equal(restore(text), source);
  });

  it("keeps brand names intact", () => {
    const source = "连接 GitHub 仓库";
    const { text, restore } = protectLiterals(source);
    assert.match(text, /\uE000\d+\uE001/);
    assert.equal(restore(text).includes("GitHub"), true);
  });
});

describe("convertMessage", () => {
  it("preserves placeholders and Trans tags", () => {
    const source = "删除 <1>{{count}}</1> 个任务";
    for (const locale of ["zh-tw", "zh-hk"]) {
      const out = convertMessage(source, locale);
      assert.match(out, /\{\{count\}\}/);
      assert.match(out, /<1>/);
      assert.match(out, /<\/1>/);
      assert.equal(out.includes("{{count}}"), true);
    }
  });

  it("does not rewrite brand names", () => {
    const source = "连接 GitHub 并保存设置";
    const tw = convertMessage(source, "zh-tw");
    const hk = convertMessage(source, "zh-hk");
    assert.match(tw, /GitHub/);
    assert.match(hk, /GitHub/);
    assert.match(tw, /設定|設定/);
    assert.match(hk, /設定/);
  });

  it("applies Taiwan vs Hong Kong glossary divergences", () => {
    const software = convertMessage("软件更新", "zh-tw");
    const softwareHk = convertMessage("软件更新", "zh-hk");
    assert.match(software, /軟體/);
    assert.match(softwareHk, /軟件/);
    assert.notEqual(software, softwareHk);

    const networkTw = convertMessage("网络错误", "zh-tw");
    const networkHk = convertMessage("网络错误", "zh-hk");
    assert.match(networkTw, /網路/);
    assert.match(networkHk, /網絡/);

    const projectTw = convertMessage("选择项目", "zh-tw");
    const projectHk = convertMessage("选择项目", "zh-hk");
    assert.match(projectTw, /專案/);
    assert.match(projectHk, /項目/);
  });

  it("maps product UI verbs via glossary", () => {
    assert.equal(convertMessage("登录", "zh-tw"), "登入");
    assert.equal(convertMessage("保存", "zh-hk"), "儲存");
    assert.equal(convertMessage("设置", "zh-tw"), "設定");
    assert.equal(convertMessage("默认", "zh-hk"), "預設");
    assert.match(convertMessage("显示语言", "zh-tw"), /顯示語言/);
  });

  it("uses region pull-request wording", () => {
    const tw = convertMessage("打开拉取请求", "zh-tw");
    const hk = convertMessage("打开拉取请求", "zh-hk");
    assert.match(tw, /提取要求/);
    assert.match(hk, /拉取要求/);
  });

  it("does not rewrite 資料 inside 資料夾 for Hong Kong", () => {
    assert.equal(convertMessage("文件夹", "zh-hk"), "資料夾");
    assert.equal(convertMessage("资料夹", "zh-hk"), "資料夾");
    assert.equal(convertMessage("文件夹", "zh-tw"), "資料夾");
  });

  it("maps filesystem 文件 to 檔案 in both regions", () => {
    assert.equal(convertMessage("文件", "zh-tw"), "檔案");
    assert.equal(convertMessage("文件", "zh-hk"), "檔案");
  });
});
