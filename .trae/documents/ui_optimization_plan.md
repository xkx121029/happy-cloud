# HappyCloud 项目 UI 优化方案

## Context（背景）

HappyCloud 是"石墨灰 + 青绿"风格的个人云盘，含 **Android(Compose)** 与 **Web(Vue3 + Naive UI)** 两端。

当前问题：
- **Android**：`MainScreen` 底部 `NavigationBar` 未显式处理系统导航栏 inset（历史反馈的 taskbar overlap）；点击登录可能崩溃（`LoginViewModel` 只捕获 `ApiException`，畸形服务器地址会让 Retrofit 抛 `IllegalArgumentException` 逃逸造成崩溃）；登录/注册页较朴素。
- **Web**：主色是**蓝色**（`--hc-primary:#2563eb`），与 Android 的**青绿**不一致，两端设计语言割裂；部分用色是蓝紫多彩渐变（admin-glow、notify-avatar 等）较俗。

**目标**：修复崩溃与重叠 bug，重做登录/注册页，并将 Web 端配色统一为 Android 端的石墨灰 + 青绿体系，提升整体精致度。

---

## 一、Android 端

### 1. 修复登录/注册点击崩溃（[LoginViewModel.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\login\LoginViewModel.kt)）
- 根因：`login()` 协程内只 `catch (e: ApiException)`。当用户输入的服务器地址不带 scheme（如 `10.0.2.2:1234`）时，`Network.api` getter 里 `Retrofit.Builder().baseUrl(...)` 抛 `IllegalArgumentException`，一路逃逸导致崩溃；`tokenStore.save*` 的 IOException 同理逃逸。
- 修复：
  - 在 `login()` 里把 catch 放宽为 `catch (e: Exception)`，统一 `_state.update { loading=false; error = 中文提示 }`。
  - 新增 URL 规范化辅助：无 scheme 时自动补 `http://`；`Network.setBaseUrl` 内对非法输入做安全兜底（仍保留默认值）。改动集中在 `LoginViewModel.kt` + `data/Network.kt` 的 `setBaseUrl`。

### 2. 修复底部导航重叠（[AppNavHost.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\AppNavHost.kt) `MainScreen`）
- 给底部 `NavigationBar` 显式加 `Modifier.navigationBarsPadding()`（或 `windowInsetsPadding(NavigationBarDefaults.windowInsets)`），并设 `containerColor = surfaceContainer`，确保在沉浸式 edge-to-edge 下不压系统导航条。
- 同样检查 `FilesScreen` 顶部的 `statusBarsPadding` 已具备（已有），保持一致。

### 3. 登录/注册页重设计（[LoginScreen.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\login\LoginScreen.kt)、[RegisterScreen.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\register\RegisterScreen.kt)）
- 保持"简洁、不落俗套"：顶部用一层**克制的青绿渐晕背景**（`Brush.verticalGradient` 从 `primaryContainer` 弱化到 `background`）。
- 品牌区改为**圆角利落图标块**（`primaryContainer` 底 + `Cloud` 图标 + 温柔阴影），下方 app 名与副标题。
- 输入框与按钮统一用 `ButtonShape`、合理的 `14dp` 间距、`titleMedium` 按钮文字；错误文案用 `error` 色。
- 两页视觉手法一致（共享风格），注册页保留返回箭头。

### 4. 精致度微调（整体）
- `Type.kt` 字体已是无衬线 + M3 Expressive，保留。
- 复核 `NavigationBar` 选中态、`EmptyState`/`ErrorState` 一致性（已有），仅按需微调间距。

---

## 二、Web 端（统一为石墨灰 + 青绿）

### 1. 统一设计 token（核心：[web/src/styles.css](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web\src\styles.css)）
将 `--hc-*` 系列与全部硬编码**蓝色/蓝紫**替换为与 Android 一致的青绿 + 石墨灰：

| token | 现在(蓝) | 改为(青绿/石墨) |
|---|---|---|
| `--hc-primary` | `#2563eb` | `#0E6A5C` |
| `--hc-primary-strong` | `#3B82F6` | `#0B564B` |
| `--hc-bg` | `#f5f6f8` | `#F7F5F1` |
| `--hc-border` | `#dfe3ea` | `#E6E3DE` |
| `--hc-border-strong` | `#d3d9e2` | `#DCD9D4` |
| `--hc-text` | `#1f2329` | `#1B1A19` |
| `--hc-text-secondary` | `#6b7280` | `#48473D` |
| `--hc-text-muted` | `#9ca3af` | `#69685E` |

- 替换硬编码青/蓝：`::selection` 的 `rgba(37,99,235,…)`、`.brand-mark` 阴影、`.drop-mask rgba(37,99,235,…)`、`.notify-card.unread`（`#eff6ff/#bfdbfe` → 青绿浅底如 `#E2F3ED`）、`.notify-card-avatar` 渐变（`#2563eb,#6366f1` → `#0E6A5C` 系）、`.sidebar-item.active`、`.batch-bar`、`.row-selected`、`.file-card.selected` 的 `rgba(37,99,235,…)`、`html.hc-dark` 相关覆盖。
- `.brand-mark.admin-glow` 的蓝紫渐变改为克制青绿处理（去掉多彩渐变俗套）。
- 兼容触屏/窄屏规则保持不变。

### 2. Naive UI 主题统一（[web/src/App.vue](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web\src\App.vue)）
- `themeOverrides.common`：`primaryColor` 等改为青绿体系；`borderRadius` 由 `6px` 适当提到与 App/材质一致（如 `8px`）；补 `fontFamily` 保持无衬线。

### 3. 深色模式对齐（`styles.css` 底部 `html.hc-dark`）
- 用 Android 深色 token：`--hc-bg:#171513`、`--hc-bg-panel:#1D1B19`、`--hc-bg-soft:#232221`、`--hc-border:#3B3E3B`、`--hc-text:#E6E4DF`、`--hc-text-secondary:#C2C4C0`、`--hc-text-muted:#9A9C98`，高亮用 `#8BD3C6`。

### 4. 认证页视觉统一（[Login.vue](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web\src\views\user\Login.vue)、Register.vue + styles.css 认证段 `.auth-*`）
- 用青绿渐变背景 + 利落圆角卡片，与 Android 登录页同一设计语言；调整 `.auth-card` 圆角/阴影到石墨灰体系。

---

## 关键文件清单
- Android：[LoginViewModel.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\login\LoginViewModel.kt)、[Network.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\data\Network.kt)、[AppNavHost.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\AppNavHost.kt)、[LoginScreen.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\login\LoginScreen.kt)、[RegisterScreen.kt](d:\xkx\xkx_appproj\happy_teach\happy_cloud\android\app\src\main\java\com\happycloud\android\ui\register\RegisterScreen.kt)
- Web：[web/src/styles.css](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web\src\styles.css)、[web/src/App.vue](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web\src\App.vue)、[web/src/views/user/Login.vue](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web\src\views\user\Login.vue)、[web/src/views/user/Register.vue](d:\xkx\xkx_appproj\happy_teach\happy_cloud\web\src\views\user\Register.vue)

## 验证
- Android：`android\gradlew.bat assembleDebug` 编译通过；真机/模拟器验证登录（含畸形地址不再崩溃）、底部导航不再压系统条、登录/注册页视觉。
- Web：`cd web && npm run build` 构建通过；本地 `npm run dev` 打开登录/注册/主页核对青绿配色与深浅模式。