---
spec: docs/specs/pwa-window-controls-overlay/spec.md
created: 2026-08-26
status: implemented
---

# 实现计划：桌面 PWA 融合标题栏

## 概述

先让现有 PWA manifest 选择浏览器的 Window Controls Overlay 模式，再由应用根 Shell 暴露
overlay 的实时几何信息。复用现有 40px 桌面侧栏、页面和任务顶栏作为融合标题栏表面，避开
系统窗口控件排除区，并使所有交互子元素退出拖动区域。不支持该能力或未安装的浏览器环境
继续使用当前布局。

## 前端

### PWA 能力契约

- `apps/web/public/manifest.webmanifest`：增加 `display_override`，将
  `window-controls-overlay` 放在首位，以 `standalone` 作为明确回退，并保留现有 `display` 字段。
- `apps/web/global.d.ts`：为 `navigator.windowControlsOverlay`、可见性、几何矩形和
  `geometrychange` 事件增加受限的 TypeScript 声明，不引入通用浏览器能力抽象。
- `apps/web/hooks/use-window-controls-overlay.ts`：增加聚焦的 hook，读取初始可见性/几何信息，
  订阅 `geometrychange`，卸载时移除监听器；API 缺失或 overlay 不可见时返回非激活状态。
- `apps/web/hooks/use-window-controls-overlay.test.tsx`：使用可控 fake overlay 覆盖 API 缺失、
  初始可见、几何变化、变为隐藏和卸载场景。
- `apps/web/lib/browser/pwa-manifest.test.ts`：固定 manifest 的 opt-in 与 `standalone` 回退，
  防止后续 manifest 清理无意恢复多余的浏览器标题栏。

### 根 Shell 与桌面应用栏

- `apps/web/src/app-shell.tsx`：只调用一次 overlay hook，在根 `h-dvh` Shell 上设置稳定的
  `data-window-controls-overlay` 状态、数值安全区 CSS 变量及稳定 E2E 锚点，不持久化任何状态。
- `apps/web/components/app-sidebar/app-sidebar-header.tsx` 和
  `apps/web/components/app-sidebar/app-sidebar.tsx`：将桌面侧栏标题区标记为融合标题栏左段；在
  侧栏展开和折叠状态下，让品牌、工作区选择器及侧栏开关避开左侧系统控件区。如果折叠标题区
  必须超出 56px rail 才能让展开操作可达，则把导航/底栏裁剪限定在各自区域，不裁剪标题区。
- `apps/web/components/page-topbar.tsx` 和 `apps/web/components/task/task-top-bar.tsx`：把两个
  共享桌面顶栏实现标记为可拖动标题栏分段，并把实时右侧安全内边距应用到页面/任务操作区域；
  交互子元素显式设为非拖动区域。
- `apps/web/app/globals.css`：仅在根节点 overlay 可见状态下应用 overlay 高度、左右安全内边距、
  拖动/非拖动区域、主题背景及溢出保护。使用 Shell 提供的几何变量；状态缺失或隐藏时保留
  当前 class 行为。
- `apps/web/components/theme-provider.tsx`：把当前解析后的应用主题同步到活动的
  `meta[name="theme-color"]`，使浏览器绘制的窗口控制区与 Kandev 主题一致；不再让该区域只
  依赖操作系统的 `prefers-color-scheme`。
- `apps/web/components/theme-provider.test.tsx`：覆盖系统浅色但应用深色、运行时切回浅色以及
  清理静态媒体条件的主题色契约。
- `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`：导航前安装确定性的
  `navigator.windowControlsOverlay` fake，验证展开/折叠侧栏的左侧控件几何、页面/任务操作的
  右侧控件几何、实时 `geometrychange`、交互控件的可点击/非拖动属性，以及 API 缺失时的桌面
  回退。使用 bounding box 关系断言，不固定平台像素。

实现继续遵循现有 `AppShell -> AppSidebar + page/task topbar` 所有权，并复用
`apps/web/e2e/tests/layout/topbar-alignment.spec.ts` 的几何断言模式。不触碰
`docs/decisions/0039-native-desktop-integration-boundary.md` 定义的 Tauri 原生集成边界。

## 测试

- **能力生命周期：** API 缺失时保持非激活；挂载时读取可见几何；`geometrychange` 更新状态；
  overlay 隐藏时清除激活几何；卸载时移除监听器。文件：
  `apps/web/hooks/use-window-controls-overlay.test.tsx`；使用可控 `EventTarget` fake 的 Vitest
  hook 测试。
- **Manifest 契约：** Window Controls Overlay 是第一个 override，且 `standalone` 继续作为
  回退。文件：`apps/web/lib/browser/pwa-manifest.test.ts`；基于文件系统的 Vitest 测试。
- **桌面回退：** overlay 未激活时，现有 40px 标题区和顶栏几何不变。文件：
  `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`；Playwright 几何断言。

## E2E 测试

- **左侧系统控件：** 给定可用矩形从左侧排除区之后开始的可见 overlay，展开和折叠侧栏的
  标题区控件保持在其右侧并且可点击。
- **右侧系统控件：** 给定可用矩形在右侧排除区之前结束的可见 overlay，页面和任务顶栏操作
  保持在其左侧。
- **实时几何：** 触发 `geometrychange` 后安全布局边界无需刷新即可移动，同时保持
  `documentElement.scrollWidth <= clientWidth`。
- **普通桌面浏览器：** API 缺失或 `visible=false` 时，根 Shell/顶栏几何与现有桌面布局一致。
- **主题同步：** 系统颜色偏好与应用主题相反时，活动 `theme-color` 仍使用应用解析后的主题
  背景；应用内切换主题后立即更新。文件：`apps/web/components/theme-provider.test.tsx`。

全部场景位于 `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`，使用桌面
`chromium` project。根据用户明确范围，不增加移动端或平板测试。

## 验证

定向 Red-Green-Refactor 命令：

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run hooks/use-window-controls-overlay.test.tsx lib/browser/pwa-manifest.test.ts
cd apps/web && pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts
```

定向检查通过后，按任务明确要求依次执行仓库级命令：

```bash
make fmt
make typecheck test lint
```

只有全部必要检查成功后，才使用显式路径暂存并创建 Conventional Commit。只有实现、审查、
验证和提交均确认完整后，才推送集成分支。

## 验证结果

Task 01 已完成：定向 Vitest 3 个文件、5 个测试通过。Task 02 已完成：新增 Playwright
用例先因缺少融合区域标记按预期失败，完成布局后 2 个场景通过，覆盖左右安全区、侧栏折叠、
实时几何更新、拖动区域和普通桌面浏览器回退；相邻顶栏 4 个回归场景也通过。`make fmt`、
全量 typecheck、Web/harness/architecture lint、CLI 测试和全部任务定向测试通过。仓库全量测试
仍有与本改动无关的本机基础线失败，详见 Task 02。

## 实现波次与并行候选

Wave 1（顺序执行）：

- [x] [Task 01：PWA overlay 能力](task-01-pwa-overlay-capability.md)

Wave 2（顺序执行，依赖 Task 01）：

- [x] [Task 02：融合桌面标题栏布局](task-02-fused-desktop-titlebar-layout.md)

Wave 3（顺序执行，依赖 Task 02）：

- [x] [Task 03：同步融合标题栏主题色](task-03-sync-titlebar-theme-color.md)

三个任务共享根 Shell 和前端契约上下文，因此在主会话中顺序执行，不计划使用 subagent。

## 风险

- 浏览器控制的标题栏矩形因操作系统而异，也可能在运行时变化；实现必须消费浏览器报告的
  几何信息，不能推断 macOS 或 Windows 控件位置。
- 折叠后的 56px 侧栏可能窄于部分左侧系统控件区，因此必须一起验证裁剪和相邻顶栏间距。
- Headless Playwright 不会启动已安装的 PWA 窗口。确定性的 overlay fake 可以证明应用几何和
  事件处理；若实施环境允许，真实已安装 PWA 的视觉检查仍是有价值的补充证据。
