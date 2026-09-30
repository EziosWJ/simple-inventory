# 登录页视觉覆盖

依据用户提供的登录页视觉稿和素材包，登录入口采用浅蓝背景、蓝青渐变、仓储插画和右侧白色登录卡片。该页面是进入管理端前的独立身份验证入口，因此不套用 Standard Form Page 的 AppShell、PageHeader 与 FormSection；复用 Input、Checkbox、Button。后台业务页面继续遵循 MASTER.md。

- 页面色彩与样式限定在 `.login-page`，样式位于 `src/pages/login.css`。
- 桌面端显示品牌、功能介绍、透明仓储插画和登录卡片；992px 以下隐藏介绍区，显示单栏表单。
- 插画保持原始宽高比，不用登录卡片位图替代表单。素材保存在 `public/login/`，来源为用户提供的 `simple-inventory-login-assets.zip`。
- 用户名、密码使用原生 label、自动填充和可关联的错误提示；密码切换、记住我与提交支持键盘操作。
- 认证 API、Zustand 状态、登录跳转及只记住用户名的存储行为沿用现有实现。
- 忘记密码与联系管理员仅显示说明，不引入密码重置 API 或虚构联系方式。
- 底部圆点仅作为视觉装饰，不声明轮播行为；不添加自动播放动画。
