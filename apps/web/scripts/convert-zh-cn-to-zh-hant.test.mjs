import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";

import { convertWeb } from "./convert-zh-cn-to-zh-hant.mjs";
import {
  convertCatalog,
  convertMessage,
  loadGlossary,
  protectLiterals,
  residualSimplifiedInCatalog,
  TARGET_LOCALES,
} from "./lib/zh-hant-convert.mjs";

const glossary = loadGlossary();
const SOURCE_DISK_HELP = "遍历数据文件夹中的每个目录";
const REVIEWED_DISK_HELP = "遍歷資料目錄中的每個目錄";
const SYSTEM_CATALOG_FILENAME = "system.json";

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

describe("glossary coverage", () => {
  it("maps every glossary source and target to the exact regional term", () => {
    for (const locale of ["zh-tw", "zh-hk"]) {
      for (const row of [...glossary.phrases, ...glossary.tokens]) {
        assert.equal(convertMessage(row.from, locale), row[locale], `${locale} source ${row.from}`);
        assert.equal(
          convertMessage(row[locale], locale),
          row[locale],
          `${locale} target ${row[locale]}`,
        );
      }
    }
  });

  it("maps every glossary source before surrounding Chinese text", () => {
    for (const locale of TARGET_LOCALES) {
      for (const row of [...glossary.phrases, ...glossary.tokens]) {
        assert.equal(
          convertMessage(`${row.from}了`, locale),
          `${row[locale]}了`,
          `${locale}: ${row.from}`,
        );
      }
    }
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

  it("uses reviewed local software vocabulary in both regions", () => {
    const samples = [
      ["添加消息队列控件链接并运行", "新增訊息佇列控制項連結並執行"],
      ["当前智能体配置", "目前代理程式設定檔"],
      ["发送代码", "傳送程式碼"],
    ];

    for (const locale of ["zh-tw", "zh-hk"]) {
      for (const [source, expected] of samples) {
        assert.equal(convertMessage(source, locale), expected, `${locale} ${source}`);
      }
    }
  });

  it("distinguishes AI agents from network proxies", () => {
    for (const locale of TARGET_LOCALES) {
      assert.equal(convertMessage("智能体配置和子智能体", locale), "代理程式設定檔和子代理程式");
      assert.equal(convertMessage("智能体工作流", locale), "代理式工作流程");
      assert.equal(convertMessage("反向代理客户端", locale), "反向代理用戶端");
      assert.equal(convertMessage("代理", locale), "代理");

      const catalog = convertCatalog({ agent: "智能体", proxy: "代理" }, locale, {
        proxy: "代理伺服器",
      });
      assert.equal(catalog.agent, "代理程式");
      assert.equal(catalog.proxy, "代理伺服器");
    }
  });

  it("normalizes mainland technical vocabulary for Traditional Chinese", () => {
    assert.equal(
      convertMessage("调用自定义提供商凭据并审阅响应", "zh-tw"),
      "呼叫自訂提供者認證資訊並審閱回應",
    );
    assert.equal(
      convertMessage("调用自定义提供商凭据并审阅响应", "zh-hk"),
      "呼叫自訂提供者認證資料並審閱回應",
    );
    assert.equal(convertMessage("磁盘进程", "zh-tw"), "磁碟處理程序");
    assert.equal(convertMessage("磁盘进程", "zh-hk"), "磁碟程序");
    assert.equal(convertMessage("显式回退标志", "zh-tw"), "明確指定的備援旗標");
    assert.equal(convertMessage("显式回退标志", "zh-hk"), "明確指定的後備旗標");
  });

  it("uses reviewed regional vocabulary in high-traffic UI copy", () => {
    assert.equal(convertMessage("生成文本列表并查看反馈", "zh-tw"), "產生文字清單並檢視意見回饋");
    assert.equal(convertMessage("生成文本列表并查看反馈", "zh-hk"), "產生文字列表並查看反饋");
    assert.equal(convertMessage("禁用组件后查看屏幕", "zh-tw"), "停用元件後檢視螢幕");
    assert.equal(convertMessage("禁用组件后查看屏幕", "zh-hk"), "停用組件後查看屏幕");
    assert.equal(convertMessage("邮箱字段和变量界面", "zh-tw"), "電子郵件地址欄位和變數介面");
    assert.equal(convertMessage("邮箱字段和变量界面", "zh-hk"), "電郵地址欄位和變量介面");
    assert.equal(
      convertMessage("查看后台克隆历史并了解详情", "zh-tw"),
      "檢視後台複製歷史並了解詳情",
    );
    assert.equal(
      convertMessage("查看后台克隆历史并了解详情", "zh-hk"),
      "查看後台複製歷史並了解詳情",
    );
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

  it("uses Traditional Chinese corner quotes for UI copy", () => {
    assert.equal(convertMessage("点击“启动智能体”", "zh-tw"), "點選「啟動代理程式」");
    assert.equal(convertMessage("点击“启动智能体”", "zh-hk"), "點擊「啓動代理程式」");
  });
});

describe("convertCatalog", () => {
  it("applies reviewed key overrides after mechanical conversion", () => {
    const converted = convertCatalog({ diskRefreshHelp: SOURCE_DISK_HELP }, "zh-tw", {
      diskRefreshHelp: REVIEWED_DISK_HELP,
    });

    assert.equal(converted.diskRefreshHelp, REVIEWED_DISK_HELP);
  });

  it("rejects reviewed overrides that do not match a catalog key", () => {
    assert.throws(
      () => convertCatalog({ known: "设置" }, "zh-tw", { typo: "設定" }),
      /unknown override key.*typo/i,
    );
  });
});

describe("catalog integrity", () => {
  it("reports malformed glossary expansion artifacts", () => {
    assert.deepEqual(
      residualSimplifiedInCatalog({
        workflow: "工作流程程",
        profile: "設定設定",
        email: "電子電子郵件地址",
        repository: "儲存儲存庫",
        history: "曆史",
        agent: "智能智能体",
        healthy: "工作流程",
      }),
      ["workflow", "profile", "email", "repository", "history", "agent"],
    );
  });

  it("rejects invalid generated catalogs before writing any file", () => {
    const root = mkdtempSync(path.join(tmpdir(), "kandev-zh-hant-"));
    const sourceDir = path.join(root, "zh-cn");
    const targetRoot = path.join(root, "output");
    mkdirSync(sourceDir, { recursive: true });
    writeFileSync(
      path.join(sourceDir, SYSTEM_CATALOG_FILENAME),
      `${JSON.stringify({ diskRefreshHelp: SOURCE_DISK_HELP })}\n`,
      "utf8",
    );

    try {
      assert.throws(
        () =>
          convertWeb({
            locales: ["zh-tw"],
            namespace: null,
            write: true,
            sourceDir,
            targetRoot,
          }),
        /refusing to write/i,
      );
      assert.equal(existsSync(path.join(targetRoot, "zh-tw", SYSTEM_CATALOG_FILENAME)), false);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("writes a catalog after a reviewed override clears the integrity issue", () => {
    const root = mkdtempSync(path.join(tmpdir(), "kandev-zh-hant-"));
    const sourceDir = path.join(root, "zh-cn");
    const targetRoot = path.join(root, "output");
    mkdirSync(sourceDir, { recursive: true });
    writeFileSync(
      path.join(sourceDir, SYSTEM_CATALOG_FILENAME),
      `${JSON.stringify({ diskRefreshHelp: SOURCE_DISK_HELP })}\n`,
      "utf8",
    );

    try {
      convertWeb({
        locales: ["zh-tw"],
        namespace: null,
        write: true,
        sourceDir,
        targetRoot,
        overrides: {
          "zh-tw": {
            system: { diskRefreshHelp: REVIEWED_DISK_HELP },
          },
        },
      });
      const written = JSON.parse(
        readFileSync(path.join(targetRoot, "zh-tw", SYSTEM_CATALOG_FILENAME), "utf8"),
      );
      assert.equal(written.diskRefreshHelp, REVIEWED_DISK_HELP);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("loads reviewed overrides in the real CLI pipeline", () => {
    const result = spawnSync(
      process.execPath,
      ["scripts/convert-zh-cn-to-zh-hant.mjs", "--locale", "all", "--namespace", "system"],
      { cwd: path.resolve("."), encoding: "utf8" },
    );

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /residual-simplified warnings: web=0 backend=0/);
  });
});
