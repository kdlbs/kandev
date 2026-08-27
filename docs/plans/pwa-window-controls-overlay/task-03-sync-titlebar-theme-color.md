---
id: "03-sync-titlebar-theme-color"
title: "同步融合标题栏主题色"
status: done
wave: 3
depends_on: ["02-fused-desktop-titlebar-layout"]
plan: "plan.md"
spec: "../../specs/pwa-window-controls-overlay/spec.md"
---

# Task 03：同步融合标题栏主题色

## 根因

`index.html` 的两个 `theme-color` 仅按操作系统 `prefers-color-scheme` 选择，而 Kandev 的主题
可以独立设置。当系统为浅色、应用为深色时，Vivaldi 等浏览器绘制的窗口控制区仍取浅色值，
因此融合后的标题栏两侧出现白边。

## 验收条件

- 浏览器活动的 `theme-color` 始终匹配 Kandev 当前解析后的浅色或深色主题。
- 应用内切换主题后无需刷新或重启 PWA 即可更新浏览器窗口控制区颜色。
- 不改变移动端或平板布局，也不增加用户可见文案。

## TDD 与验证

先在 `apps/web/components/theme-provider.test.tsx` 写入系统浅色但应用深色的回归场景，并确认
当前实现仍保留浅色 `theme-color` 而按预期 RED。完成最小实现后执行：

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/theme-provider.test.tsx
cd apps/web && pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts
```

随后执行任务要求的仓库级命令：

```bash
make fmt
make typecheck test lint
```

## 可能修改的文件

- `apps/web/index.html`
- `apps/web/components/theme-provider.tsx`
- `apps/web/components/theme-provider.test.tsx`
- `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`
- `docs/specs/pwa-window-controls-overlay/spec.md`
- `docs/plans/pwa-window-controls-overlay/plan.md`
- `docs/plans/pwa-window-controls-overlay/task-03-sync-titlebar-theme-color.md`

## 输出契约

记录 RED/GREEN 证据、真实 Vivaldi PWA 深色视觉复验、定向测试及仓库级验证结果；只改 PC
融合标题栏相关契约。

## 结果

- RED：`pnpm --filter @kandev/web test -- --run components/theme-provider.test.tsx` 按预期失败，
  深色应用主题下仍存在两个按系统 `prefers-color-scheme` 选择的 `theme-color`。
- GREEN：格式化后的最终 Vitest 为 2 个文件、3 个测试通过，覆盖系统浅色与应用深色相反、
  深色 `#181818`、运行时切回浅色 `#ffffff` 以及静态媒体条件清理。
- Playwright 生产构建的桌面 `chromium` project 为 2 个场景全部通过；融合场景明确模拟系统
  浅色、应用深色和可见 Overlay API，并验证活动 `theme-color` 为 `#181818`。
- `make fmt` 使用仓库锁定的 `pnpm@9.15.9` 执行后通过；全量 typecheck 通过，Web lint 通过。
- `make typecheck test lint` 中全量后端测试仍受本机既有环境/基础线问题影响，包括 npm cache
  根路径、ACP probe、Git 中文输出和安装路径元数据；后端 lint 因本机缺少 `golangci-lint`
  未启动。这些失败均不在本任务修改路径内。
- 用户的二级 Vivaldi PWA 实例仍在 `http://localhost:18702` 运行，Vite 已接收
  `theme-provider.tsx` HMR 更新；最终宿主窗口视觉颜色由用户在 Test Phase 复验。
- 仅在 `navigator.windowControlsOverlay` 存在时同步宿主主题色，未修改移动端或平板布局。
