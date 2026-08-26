---
id: "01-pwa-overlay-capability"
title: "PWA overlay 能力"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/pwa-window-controls-overlay/spec.md"
---

# Task 01：PWA overlay 能力

## 验收条件

- Web app manifest 首先请求 `window-controls-overlay`，并保留 `standalone` 作为不支持该能力的
  浏览器回退。
- 一个由根 Shell 拥有的 hook 报告激活标题栏几何，响应可见性/几何变化，并在卸载时移除
  监听器；能力缺失时保持稳定的非激活状态。
- `AppShell` 通过根作用域 CSS 变量发布当前 overlay 状态与安全几何，不改变普通浏览器布局，
  也不持久化浏览器能力状态。

## 验证

先在生产代码修改前运行并确认 RED，再在 GREEN 和 REFACTOR 后重跑：

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run hooks/use-window-controls-overlay.test.tsx lib/browser/pwa-manifest.test.ts
```

## 可能修改的文件

- `apps/web/public/manifest.webmanifest`
- `apps/web/global.d.ts`
- `apps/web/hooks/use-window-controls-overlay.ts`
- `apps/web/hooks/use-window-controls-overlay.test.tsx`
- `apps/web/lib/browser/pwa-manifest.test.ts`
- `apps/web/src/app-shell.tsx`

## 依赖

无。

## 并行性

顺序执行。Task 02 消费本任务创建的根 overlay 状态和 CSS 变量契约。

## 输入

- Spec 的 `What` 和 `Scenarios`
- Plan 的 `PWA 能力契约` 和 `测试`
- `apps/web/src/app-shell.tsx`
- `apps/web/public/manifest.webmanifest`

## 风险

- 测试必须因行为断言失败，不能只因新模块尚不存在而编译失败。
- 事件类型声明只覆盖 Kandev 实际消费的浏览器 API。

## 输出契约

在同一会话中报告 RED/GREEN 命令结果、变更文件、最终 hook 契约、阻塞/风险，并同步更新本
任务和 `plan.md` 的状态与结果。

## 结果

- RED：`npx --yes pnpm@9.15.9 --filter @kandev/web test -- --run src/app-shell.test.tsx lib/browser/pwa-manifest.test.ts`，2 个行为断言按预期失败；先修正 manifest 测试的文件路径后，失败分别为缺少 `app-shell` overlay 契约和缺少 `display_override`。
- RED：`npx --yes pnpm@9.15.9 --filter @kandev/web test -- --run hooks/use-window-controls-overlay.test.tsx`，2 个测试按预期因未订阅/移除 `geometrychange` 失败。
- GREEN：`npx --yes pnpm@9.15.9 --filter @kandev/web test -- --run hooks/use-window-controls-overlay.test.tsx src/app-shell.test.tsx lib/browser/pwa-manifest.test.ts`，3 个文件、5 个测试通过。
- 实现了 manifest opt-in、受限浏览器类型、根 Shell overlay 几何变量和可见性生命周期；无外部副作用。
