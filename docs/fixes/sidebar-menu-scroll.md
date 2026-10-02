# 修复后台侧边菜单过多时无法滚动

**GitHub issue:** [#38](https://github.com/EziosWJ/simple-inventory/issues/38)

## What to build

后台侧边菜单总高度超过窗口可用高度时，用户能够在菜单区域通过滚轮滚动查看并点击最底部菜单。顶部产品 Logo 保持固定，主内容区域继续独立滚动。侧边栏展开、折叠及分组展开后均适用。

## Problem

真实侧边栏组件的浏览器复现已确认：600px 高窗口中，20 个测试菜单加默认菜单后，最后一项位于 1004～1044px，滚轮操作后位置不变。固定侧边栏和随内容撑高的菜单区均未形成受高度约束的滚动容器。仅给菜单区启用纵向 overflow 仍不能滚动；限制菜单区为顶部 Logo 以下的剩余高度并启用滚动后可正常查看底部。

## Acceptance criteria

- [x] 菜单内容超过可用高度时，滚轮能滚动到最后一项，并可点击完成路由导航。
- [x] 顶部 Logo 保持原高度和位置，菜单滚动不带动 Logo 或主内容区。
- [x] 侧边栏展开和折叠模式、展开长子菜单、调整窗口高度后，底部菜单均可到达。
- [x] 菜单较少时布局正常，无多余滚动条；保持现有桌面可见与小屏隐藏规则。
- [x] 用真实侧边栏组件完成浏览器回归验证，并运行前端 lint/build。

## Blocked by

None (can start immediately).

## Scope

仅修复后台侧边栏布局与滚动，不涉及 API、权限菜单数据、数据库或进销存业务规则。

## Implementation and validation

本地实现完成：侧边栏使用纵向 flex 布局，Logo 不收缩，菜单区占剩余高度并允许纵向滚动；滚动到边界时不把滚轮传递到主内容区。

- 修复前真实组件回归检查失败：滚轮后菜单区 scrollTop 始终为 0。
- 修复后 `npm run test:sidebar-scroll` 通过：展开、折叠及切回展开、展开长子菜单、窗口缩小、短菜单、小屏隐藏，并确认底部菜单点击后路由正确。
- `task frontend:lint`、`task frontend:build` 通过。
- 浏览器检查支持使用 `PLAYWRIGHT_EXECUTABLE_PATH` 指定已安装的 Chromium。
- 仅前端改动，未执行后端或数据库检查。

线上 issue 保持开启，待修复提交推送并完成代码交付后关闭。
