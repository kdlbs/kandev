# Chinese UI glossary: zh-cn → zh-tw / zh-hk

Source of truth for **product vocabulary** when converting Simplified Chinese
catalogs to Traditional Chinese (Taiwan / Hong Kong). Character conversion
(OpenCC `s2twp` / `s2hk`) handles most glyphs; this file covers terms where:

- region software norms differ from bare conversion;
- Taiwan and Hong Kong diverge from each other;
- Kandev product nouns must stay consistent across namespaces.

## How to apply

1. Convert each `zh-cn` string with OpenCC (`s2twp` → `zh-tw`, `s2hk` → `zh-hk`).
2. Apply **exact whole-word / phrase replacements** from the tables below
   (longest match first) on the converted text.
3. Do **not** rewrite brand names, code identifiers, URLs, `{{interpolation}}`
   placeholders, or HTML-ish `<n>` Trans tags.
4. When a new product term appears in `zh-cn`, add a row here in the same PR
   that introduces the Traditional update.

Priority when rows conflict: **longer phrase wins**, then **more specific
domain table** over general UI.

---

## Brands and never-translate

| Keep as-is |
|---|
| Kandev |
| GitHub / GitLab / Jira / Linear / Azure DevOps / Sentry |
| PR / MR (when used as Latin abbreviations next to full forms) |
| JSON / YAML / CLI / API / URL / SSH / HTTP / MCP / ACP |
| Type-to-confirm tokens compared with `===` (e.g. `DELETE ALL`) |

---

## Core product nouns

| English concept | zh-cn | zh-tw | zh-hk | Notes |
|---|---|---|---|---|
| workspace | 工作区 | 工作區 | 工作區 | OpenCC usually enough |
| task | 任务 | 任務 | 任務 | |
| session | 会话 | 工作階段 | 工作階段 | Prefer 工作階段 over 會話 for UI |
| agent | 代理 / Agent | 代理 / Agent | 代理 / Agent | Keep product sense; do not use 經紀人 |
| executor | 执行器 | 執行器 | 執行器 | |
| workflow | 工作流 | 工作流程 | 工作流程 | Prefer 工作流程 in UI |
| automation | 自动化 | 自動化 | 自動化 | |
| repository / repo | 仓库 | 儲存庫 | 儲存庫 | Avoid 倉庫 for git repos |
| pull request | 拉取请求 | 提取要求 | 拉取要求 | GitHub TW often 提取要求; HK 拉取要求 |
| merge request | 合并请求 | 合併請求 | 合併請求 | GitLab |
| branch | 分支 | 分支 | 分支 | |
| commit | 提交 | 提交 | 提交 | Noun/verb both 提交 in git UI |
| diff / changes | 变更 / 更改 | 變更 / 更改 | 變更 / 更改 | |
| review | 审查 / 评审 | 審查 / 評審 | 審查 / 評審 | Prefer 審查 for code review |
| plugin | 插件 | 外掛 | 外掛程式 | TW 外掛; HK often 外掛程式 |
| office | 办公室 / Office | 辦公室 / Office | 辦公室 / Office | Product area name |
| kanban | 看板 | 看板 | 看板 | |

---

## General software UI

| English concept | zh-cn | zh-tw | zh-hk | Notes |
|---|---|---|---|---|
| software | 软件 | 軟體 | 軟件 | Classic TW/HK split |
| network | 网络 | 網路 | 網絡 | |
| information | 信息 | 資訊 | 資訊 | |
| data | 数据 | 資料 | 數據 | HK often keeps 數據 |
| file | 文件 | 檔案 | 檔案 | When meaning filesystem file |
| document | 文档 | 文件 | 文件 | When meaning written doc |
| folder | 文件夹 | 資料夾 | 資料夾 | |
| settings | 设置 | 設定 | 設定 | |
| default | 默认 | 預設 | 預設 | |
| user | 用户 | 使用者 | 用戶 | TW UI often 使用者 |
| account | 账户 / 账号 | 帳戶 | 帳戶 | |
| login / sign in | 登录 | 登入 | 登入 | |
| logout / sign out | 退出登录 / 登出 | 登出 | 登出 | |
| save | 保存 | 儲存 | 儲存 | |
| create | 创建 | 建立 | 建立 | Prefer 建立 over 創建 in buttons |
| delete | 删除 | 刪除 | 刪除 | OpenCC |
| search | 搜索 | 搜尋 | 搜尋 | |
| load / loading | 加载 | 載入 | 載入 | |
| upload | 上传 | 上傳 | 上傳 | |
| download | 下载 | 下載 | 下載 | |
| server | 服务器 | 伺服器 | 伺服器 | |
| project | 项目 | 專案 | 項目 | TW 專案 / HK 項目 |
| quality | 质量 | 品質 | 質素 / 品質 | Prefer 品質 in product UI for both unless copy is about media quality |
| video | 视频 | 影片 | 影片 | Prefer 影片 in both product UIs |
| memory (RAM) | 内存 | 記憶體 | 記憶體 | |
| cache | 缓存 | 快取 | 快取 | |
| paste | 粘贴 | 貼上 | 貼上 | |
| copy | 复制 | 複製 | 複製 | |
| print | 打印 | 列印 | 列印 | |
| email | 邮件 / 邮箱 | 電子郵件 / 電郵 | 電郵 / 電郵地址 | Prefer 電子郵件 for formal; 電郵 OK short |
| permission | 权限 | 權限 | 權限 | |
| role | 角色 | 角色 | 角色 | |
| token | 令牌 / Token | 權杖 / Token | 權杖 / Token | Prefer keeping `Token` where API-facing |
| password | 密码 | 密碼 | 密碼 | |
| error | 错误 | 錯誤 | 錯誤 | |
| warning | 警告 | 警告 | 警告 | |
| success | 成功 | 成功 | 成功 | |
| failed / failure | 失败 | 失敗 | 失敗 | |
| cancel | 取消 | 取消 | 取消 | |
| confirm / OK | 确定 | 確定 | 確定 | |
| close | 关闭 | 關閉 | 關閉 | |
| open | 打开 | 開啟 | 開啟 | Prefer 開啟 for actions |
| edit | 编辑 | 編輯 | 編輯 | |
| export | 导出 | 匯出 | 匯出 | |
| import | 导入 | 匯入 | 匯入 | |
| language | 语言 | 語言 | 語言 | |
| display language | 显示语言 | 顯示語言 | 顯示語言 | |

---

## Git / VCS chrome (high visibility)

| English concept | zh-cn | zh-tw | zh-hk |
|---|---|---|---|
| stage | 暂存 | 暫存 | 暫存 |
| unstage | 取消暂存 | 取消暫存 | 取消暫存 |
| stash | 贮藏 / Stash | 收藏 / Stash | 貯存 / Stash |
| push | 推送 | 推送 | 推送 |
| pull | 拉取 | 拉取 | 拉取 |
| fetch | 获取 | 擷取 | 擷取 |
| force push | 强制推送 | 強制推送 | 強制推送 |
| upstream | 上游 | 上游 | 上游 |
| remote | 远程 | 遠端 | 遠端 |
| conflict | 冲突 | 衝突 | 衝突 |
| resolve | 解决 | 解決 | 解決 |

---

## Auth and identity

| English concept | zh-cn | zh-tw | zh-hk |
|---|---|---|---|
| authentication | 身份验证 / 认证 | 驗證 | 驗證 |
| authorization | 授权 | 授權 | 授權 |
| personal access token | 个人访问令牌 | 個人存取權杖 | 個人存取權杖 |
| OAuth | OAuth | OAuth | OAuth |

---

## Application rules for the generator

1. **Whole-phrase table first** (product nouns, multi-character UI verbs), then
   single-token rows.
2. After glossary pass, assert no Simplified-only code points remain in
   `zh-tw` / `zh-hk` values (except Latin/brands/placeholders). Use a Unicode
   script check in the conversion script.
3. Plurals: Chinese uses the same form for `_one` / `_other` in most UI
   strings; keep both keys and convert both values.
4. Interpolation: never translate or break `{{name}}`, `$t(...)`, or `<1>…</1>`.
5. When TW and HK share a value, still write both catalogs explicitly (no
   runtime aliasing).

---

## Known residual judgment calls

These may need human review after conversion rather than hard rules:

- 「信息」in non-UI error dumps that should stay technical.
- 「文件」when the English source means "document" vs "file" — check `en` key.
- 「项目」in Azure DevOps / Jira "Project" vs generic "item".
- Length of Traditional strings in dense toolbars (truncation risk).

Record any new judgment in this file when resolved.
