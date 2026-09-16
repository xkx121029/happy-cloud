# Happy-Cloud 手机端 UI 彻底重构方案

## Context（背景与目标）
用户希望彻底重构 Android 云盘客户端「Happy-Cloud」的 UI。经确认设计方向为：
- **主视觉基调**：石墨灰(graphite)中性色底 + 单一克制青绿(muted teal)功能强调色，避免俗套 AI 审美。
- **范围**：全量重做所有页面，统一设计语言。
- **设计品味**（参考 emilkowalski / Linear / Sonner）：极简克制、留白、用细描边(hairline border)与 surfaceContainer 层次替代大面积重卡片、颜色克制、动画克制、精致排版。

**约束**：纯 UI 层纯重构。数据层 / ViewModel / ApiService / 各 Composable 签名与回调参数全部不动，不新增第三方依赖。界面文案保持中文、无衬线字体、嵌入弹窗优于系统弹窗。

当前技术栈：Kotlin + Compose + Material3 (BOM 2024.12.01，支持 M3 Expressive)，Navigation-Compose。

## 实施步骤与改动要点

### ① 主题色板：`ui/theme/Color.kt`
只**替换色值常量、保留变量名**，`Theme.kt` 逻辑不动，`LightColors/DarkColors` 自动继承。

- **Light（暖石墨灰底 + 低饱和青绿强调）**
  - primary `#11695F`、onPrimary 白、primaryContainer `#AFEFDC`、onPrimaryContainer `#00201E`
  - secondary `#56625D`/白/`#DAE7E1`/`#141F1B`；tertiary `#5B6461`/白/`#DFE9E5`/`#181D1C`
  - error 沿用 M3 标准 `#BA1A1A` 系列
  - background/surface `#F6F4F0`、onBackground/onSurface `#1A1A19`
  - surfaceVariant `#DEDBD6`、onSurfaceVariant `#53524E`、outline `#76766F`、outlineVariant `#C8C6C0`
  - surfaceContainerLowest `#FFFFFF`、Low `#F0EEEA`、Container `#E8E6E2`、High `#DEDCD9`、Highest `#D6D4D1`、surfaceTint `#11695F`
- **Dark（暖墨黑底 + 青绿提亮）**
  - primary `#8BD3C6`、onPrimary `#004039`、primaryContainer `#025047`、onPrimaryContainer `#AFEFDC`
  - secondary `#BECBC5`/`#28342F`/`#3E4B45`/`#DAE7E1`；tertiary `#C2CCC8`/`#2C3431`/`#424C49`/`#DFE9E5`
  - background/surface `#171513`、onSurface `#E6E4DF`、surfaceVariant `#3C3F3D`、onSurfaceVariant `#C2C4C0`、outline `#9A9C98`、outlineVariant `#3B3E3B`
  - surfaceContainerLowest `#11100F`、Low `#171513`、Container `#1D1B19`、High `#232221`、Highest `#2B2A28`、surfaceTint `#8BD3C6`

对比度均已满足 WCAG AA（onSurface on background≈17:1；primary on white=5.9:1）。新色板与现有硬编码（收藏金 `#F5B301`、preview 黑 `#121212`、CategoryColor）无冲突。

### ② 圆角收敛：`ui/theme/Shape.kt`
emilkowalski 风格偏"接近直角+轻微弧"。圆角整体收敛一档，减少幼态感：
```kotlin
extraSmall 8dp, small 12dp, medium 16dp, large 20dp, extraLarge 24dp
ButtonShape 14dp（原18），FabShape 16dp（原20）
新增 val DialogShape = RoundedCornerShape(22.dp)  // 对话框统一样式
```

### ③ 共享空态组件：新增 `ui/components/EmptyState.kt`
新增 `EmptyState(icon, title, subtitle, actionLabel?, onAction?)` 与 `ErrorState(message, onRetry)`，收敛 Files/Upload/Trash/Shares 四处冗余的空态/错误态。

### ④ 文件页：`ui/files/FilesScreen.kt`（改动最大，重点）
- 顶部结构：把 `SourceSwitcher + 面包屑 + ViewModeSwitch + QuotaBar + OverviewCard` 收敛为同一 `surfaceContainerLow` 面板，底部用 `HorizontalDivider(outlineVariant)` 与内容区隔离（hairline 分隔）。
- SegmentedButton：选中底 `surfaceContainerHighest` + primary 文本；收紧 padding。
- `QuotaBar`/`OverviewCard`：由重卡片改为 `surfaceContainerLow + BorderStroke(1dp, outlineVariant)` 描边轻卡片；进度条高度 6dp，trackColor `surfaceContainerHighest`。
- 抽共享「文件类型图标 + 元数据」：网格卡用 `surfaceContainerLowest + 1dp outlineVariant` 描边、`shapes.medium`；列表行用描边轻行、`shapes.large`。文件夹图标 `primary`、普通文件 `onSurfaceVariant`。
- `CategoryColor` 收敛为青绿色相明度阶梯 + 灰阶（image `#11695F`、video `#4C8C84`、audio `#7A9C97`、doc `#B0B3AE`、other `#D6D4D1`）。
- 占位 Loading/Error/Empty 就地重塑，统一 `ButtonShape`。
- 保留 `combinedClickable`、`PullToRefreshBox`、`DropdownMenu` 及各 Dialog 触发逻辑不变，只换材质与色。

### ⑤ 对话框统一
- 所有 `AlertDialog` 的 `shape = MaterialTheme.shapes.extraLarge` → 统一 **`DialogShape`(22dp)**；按钮统一 `ButtonShape`。
- 危险操作（彻底删除/清空/取消分享）主按钮用 `errorContainer`/`onErrorContainer` 克制红；常规用 `primary`。
- 涉及文件：`ui/files/FilesDialogs.kt`、`ui/trash/TrashScreen.kt`、`ui/shares/SharesScreen.kt`、`ui/share/ShareDialog.kt`。

### ⑥ 登录 / 注册：`ui/login/LoginScreen.kt`、`ui/register/RegisterScreen.kt`
- 保留 `displaySmall` 大标题 Hero + 青绿 primary Logo；`OutlinedTextField` 与主按钮圆角同步新 `ButtonShape`；背景中性 `surface`。
- 纯视觉，逻辑不动。

### ⑦ 上传任务：`ui/upload/UploadScreen.kt`
- `RunningTaskCard`/`FinishedTaskCard` 改为描边轻卡片；行内进度条 6dp；按钮 `ButtonShape`；空态复用共享 EmptyState 风格。

### ⑧ 我的：`ui/me/MeScreen.kt`
- 账号卡片改 `surfaceContainerLow + outlineVariant` 描边；`EntryRow` 图标底改 `secondaryContainer/onSecondaryContainer`；分隔线 `outlineVariant`。
- 退出登录可改 `OutlinedButton` 弱化破坏感（可选）。

### ⑨ 回收站 / 分享 / 预览
- `TrashScreen`/`SharesScreen`：TopAppBar 保留，行改描边轻卡片，TextButton 用 labelLarge+primary，空态复用共享组件。
- `PreviewDialog`：保持全黑背景与 `#E8E8E8` 文本不变，确认错误提示中文。

## 关键文件
- `android/app/src/main/java/com/happycloud/android/ui/theme/Color.kt`（① 色板）
- `android/app/src/main/java/com/happycloud/android/ui/theme/Shape.kt`（② 圆角）
- `android/app/src/main/java/com/happycloud/android/ui/files/FilesScreen.kt`（④ 文件页，改动最大）
- 新增 `android/app/src/main/java/com/happycloud/android/ui/components/EmptyState.kt`（③ 共享空态）
- `ui/files/FilesDialogs.kt`、`ui/login/LoginScreen.kt`、`ui/register/RegisterScreen.kt`、`ui/upload/UploadScreen.kt`、`ui/me/MeScreen.kt`、`ui/trash/TrashScreen.kt`、`ui/shares/SharesScreen.kt`、`ui/share/ShareDialog.kt`

## 执行顺序
① Color.kt → ② Shape.kt → ③ EmptyState.kt → ④ FilesScreen → ⑤ 对话框 → ⑥ 登录/注册 → ⑦ 上传 → ⑧ 我的 → ⑨ 回收站/分享/预览

## 验证
1. `cd android && ./gradlew assembleDebug` 编译通过（不新增依赖，仅 UI 改动）。
2. 安装 `app/build/outputs/apk/debug/app-debug.apk`，人工走查：明暗两套主题切换、文件页两视图、下拉刷新、长按菜单、四个对话框圆角、回收站/分享/上传/我的各页高度与描边统一、预览沉浸黑。
3. 复查无残留旧暖陶土色（其余硬编码仅剩收藏金、预览黑、CategoryColor 应收敛）。