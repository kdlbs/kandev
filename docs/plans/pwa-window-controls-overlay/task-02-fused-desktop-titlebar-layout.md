---
id: "02-fused-desktop-titlebar-layout"
title: "融合桌面标题栏布局"
status: done
wave: 2
depends_on: ["01-pwa-overlay-capability"]
plan: "plan.md"
spec: "../../specs/pwa-window-controls-overlay/spec.md"
---

# Task 02：融合桌面标题栏布局

## 验收条件

- overlay 可见时，共享侧栏/页面/任务顶部应用栏成为可拖动标题栏行；全部交互子元素均为非
  拖动区域并且保持可操作。
- 左侧或右侧系统控件排除区不会覆盖 Kandev 控件，包括展开或折叠侧栏；几何变化不会引入
  横向溢出。
- API 缺失或 `visible=false` 的桌面环境保留现有 40px 顶部应用栏；移动端和平板代码路径不变。

## 验证

先编写 Playwright 场景，并在修改生产 CSS/组件前确认它们针对当前布局得到预期 RED。GREEN
和 REFACTOR 后执行：

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts
```

随后在仓库根目录执行任务要求的检查：

```bash
make fmt
make typecheck test lint
```

每次最终生产代码或测试修改后，都重新运行受影响的定向命令再记录结果。

## 可能修改的文件

- `apps/web/components/app-sidebar/app-sidebar-header.tsx`
- `apps/web/components/app-sidebar/app-sidebar.tsx`
- `apps/web/components/page-topbar.tsx`
- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/layout/pwa-window-controls-overlay.spec.ts`
- `docs/plans/pwa-window-controls-overlay/plan.md`
- `docs/plans/pwa-window-controls-overlay/task-02-fused-desktop-titlebar-layout.md`

## 依赖

Task 01 必须先提供根 overlay 状态和几何变量契约。

## 并行性

顺序执行。桌面应用栏组件和 E2E 几何断言共享同一个布局契约。

## 输入

- 全部 Spec 场景
- Plan 的 `根 Shell 与桌面应用栏`、`E2E 测试` 和 `风险`
- `apps/web/e2e/tests/layout/topbar-alignment.spec.ts`
- `apps/web/e2e/tests/layout/task-topbar-long-title.spec.ts`
- `docs/decisions/0039-native-desktop-integration-boundary.md`

## 风险

- 不使用固定的操作系统控件偏移，必须消费 overlay 矩形。
- 应用左侧安全内边距后，不得让 `overflow-hidden` 裁掉折叠侧栏的展开操作。
- Fake API 能证明前端行为，但不能证明特定已安装浏览器暴露该平台能力；真实已安装 PWA 的
  视觉检查需要单独记录。

## 输出契约

报告 RED/GREEN E2E 证据、bounding box 关系、完整验证结果、变更文件、真实 PWA 视觉证据或
未执行的明确原因、阻塞/风险，并更新任务和计划结果。

## 结果

- RED：`pnpm e2e:run tests/layout/pwa-window-controls-overlay.spec.ts` 构建成功后，融合场景按
  预期因找不到 `[data-window-controls-overlay-region="sidebar"]` 失败；无 Overlay API 的桌面
  回退场景已通过。
- GREEN：同一命令的 2 个 Chromium 场景全部通过，验证了左侧安全区、右侧安全区、侧栏折叠、
  `geometrychange`、拖动/非拖动区域及无横向溢出。
- 侧栏标题区移出导航内容的裁剪容器，折叠时可在 56px rail 外安全容纳系统按钮与展开操作；
  页面和任务共享顶栏消费同一组根几何变量。
- 用户明确只要求 PC 端，因此未增加或修改移动端和平板布局测试。
- Headless Playwright 使用确定性 fake 验证应用侧契约；真实已安装 PWA 的视觉检查留待 Test
  Phase 在本机安装后完成。
- 格式化后的最终定向验证：Vitest 3 个文件、5 个测试通过；本任务 Playwright 2 个场景通过；
  `topbar-alignment` 与 `task-topbar-long-title` 共 4 个相邻回归场景通过。
- 仓库检查：`make fmt` 通过；`make typecheck test lint` 中全量 typecheck 通过，但后端测试因
  多个与本任务无关的本机环境/基础线问题失败，包括临时路径安全检查、ACP provider 状态、
  SQLite 锁、Git 本地化输出和本机安装路径。全量 Web Vitest 为 1505 个文件通过、1 个文件
  失败（12603 个测试通过、3 个失败、4 个跳过），唯一失败文件
  `lib/http-git-server.test.ts` 需要当前未运行的 Docker daemon。
- 分层补验：Web、harness、architecture lint 通过；CLI 5 个测试通过；脚本 Python 9 个测试
  通过，但 `scripts/pr-state` 的 `cursor_args[@]: unbound variable` 使脚本套件失败。后端 lint
  未启动，因为本机缺少 `golangci-lint`。这些失败均不在本任务修改路径内，未改写或掩盖。
