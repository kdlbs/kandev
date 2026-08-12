/**
 * Convert Simplified Chinese UI strings to Traditional Chinese (Taiwan / Hong Kong).
 *
 * Pipeline: protect placeholders/brands → OpenCC (s2twp / s2hk) → glossary
 * phrase/token overrides (longest first) → restore protections.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import * as OpenCC from "opencc-js";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const GLOSSARY_PATH = path.join(HERE, "zh-hant-glossary.json");

export const TARGET_LOCALES = ["zh-tw", "zh-hk"];

const OPENCC_TO = {
  "zh-tw": "twp",
  "zh-hk": "hk",
};

/** @type {{ brands: string[], phrases: Array<{from:string,"zh-tw":string,"zh-hk":string}>, tokens: Array<{from:string,"zh-tw":string,"zh-hk":string}> }} */
let glossaryCache;

export function loadGlossary(glossaryPath = GLOSSARY_PATH) {
  glossaryCache = JSON.parse(fs.readFileSync(glossaryPath, "utf8"));
  return glossaryCache;
}

function glossary() {
  if (!glossaryCache) loadGlossary();
  return glossaryCache;
}

const converters = new Map();

function converterFor(locale) {
  if (!converters.has(locale)) {
    const to = OPENCC_TO[locale];
    if (!to) throw new Error(`unsupported target locale: ${locale}`);
    converters.set(locale, OpenCC.Converter({ from: "cn", to }));
  }
  return converters.get(locale);
}

/**
 * Protect spans that must not be rewritten: {{placeholders}}, <n> tags, brands.
 * Returns { text, restore }.
 */
export function protectLiterals(message, brands = glossary().brands) {
  const slots = [];
  const push = (value) => {
    const token = `\uE000${slots.length}\uE001`;
    slots.push(value);
    return token;
  };

  let text = message;
  // Interpolation first so brands inside values are not an issue.
  text = text.replace(/\{\{[^}]+\}\}/g, (match) => push(match));
  text = text.replace(/<\/?\d+>/g, (match) => push(match));

  // Longer brands first so "Azure DevOps" wins over "Azure".
  const ordered = [...brands].sort((a, b) => b.length - a.length);
  for (const brand of ordered) {
    if (!brand) continue;
    // Escape regex metacharacters in brand names.
    const pattern = brand.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    text = text.replace(new RegExp(pattern, "g"), () => push(brand));
  }

  return {
    text,
    restore(converted) {
      return converted.replace(/\uE000(\d+)\uE001/g, (_, index) => slots[Number(index)] ?? "");
    },
  };
}

function applyGlossary(text, locale) {
  const { phrases, tokens } = glossary();
  const rows = [...phrases, ...tokens].sort((a, b) => b.from.length - a.from.length);
  const slots = [];
  let out = text;
  for (const row of rows) {
    const replacement = row[locale];
    if (!replacement || !row.from) continue;
    if (!out.includes(row.from)) continue;
    // Protect even when the form is unchanged so a shorter token cannot rewrite
    // inside a longer phrase (e.g. 資料 inside 資料夾 → 數據夾 on zh-hk).
    const token = `\uE010${slots.length}\uE011`;
    slots.push(replacement);
    out = out.split(row.from).join(token);
  }
  return out.replace(/\uE010(\d+)\uE011/g, (_, index) => slots[Number(index)] ?? "");
}

/**
 * Convert one message string to the target Traditional locale.
 * Applies glossary before and after OpenCC so both simplified source forms and
 * OpenCC traditional output can hit region overrides.
 */
export function convertMessage(message, locale) {
  if (typeof message !== "string" || message.length === 0) return message;
  const { text, restore } = protectLiterals(message);
  const preGlossed = applyGlossary(text, locale);
  const converted = converterFor(locale)(preGlossed);
  const postGlossed = applyGlossary(converted, locale);
  return restore(postGlossed);
}

/** Convert a flat key→string catalog object. */
export function convertCatalog(messages, locale) {
  const out = {};
  for (const [key, value] of Object.entries(messages)) {
    out[key] = typeof value === "string" ? convertMessage(value, locale) : value;
  }
  return out;
}

/**
 * Residual Simplified multi-character forms that should not remain after
 * conversion. Whole phrases only — character classes false-positive on shared
 * glyphs (e.g. 任 in both 任务 and 任務).
 */
const SIMPLIFIED_MARKERS =
  /软件|网络|信息|数据|文件夹|设置|默认|用户|登录|加载|服务器|项目|质量|视频|内存|缓存|粘贴|复制|打印|权限|密码|错误|失败|确定|关闭|打开|编辑|导出|导入|语言|会话|仓库|插件|评审|暂存|远程|冲突|解决|获取|认证|授权|任务|执行器|自动化|创建|搜索|保存|账户|账号|拉取请求|合并请求|显示语言/;

export function hasResidualSimplified(message) {
  if (typeof message !== "string") return false;
  const { text } = protectLiterals(message);
  return SIMPLIFIED_MARKERS.test(text);
}

export function residualSimplifiedInCatalog(messages) {
  const hits = [];
  for (const [key, value] of Object.entries(messages)) {
    if (hasResidualSimplified(value)) hits.push(key);
  }
  return hits;
}
