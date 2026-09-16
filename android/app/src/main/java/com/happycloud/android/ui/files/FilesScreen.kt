package com.happycloud.android.ui.files

import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CreateNewFolder
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.FolderOpen
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.StarBorder
import androidx.compose.material.icons.filled.Upload
import androidx.compose.material.icons.filled.ViewList
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.happycloud.android.AppContainer
import com.happycloud.android.data.FileItem
import com.happycloud.android.data.StorageOverview
import com.happycloud.android.ui.components.EmptyState
import com.happycloud.android.ui.components.ErrorState
import com.happycloud.android.ui.preview.PreviewDialog
import com.happycloud.android.ui.share.ShareDialog
import com.happycloud.android.util.FormatUtil
import com.happycloud.android.util.PreviewUtil

/** 收藏星标激活色（降饱和金，与石墨/青绿底色更协调） */
private val FAVORITE_ACTIVE = Color(0xFFE0A800)

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun FilesScreen(
    modifier: Modifier = Modifier,
    viewModel: FilesViewModel,
    container: AppContainer,
) {
    val state by viewModel.state.collectAsState()
    val context = LocalContext.current
    val snackbarHostState = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val p2pMode by container.p2p.mode.collectAsState()
    val p2pStatus by container.p2p.status.collectAsState()

    // 页面加载后尝试建立 P2P 打洞隧道（仅触发一次，失败不影响 HTTP 直连）
    LaunchedEffect(Unit) {
        container.p2p.ensureConnected(scope)
    }

    // 菜单与对话框状态
    var menuTarget by remember { mutableStateOf<FileItem?>(null) }
    var showNewFolder by remember { mutableStateOf(false) }
    var showRename by remember { mutableStateOf<FileItem?>(null) }
    var showMove by remember { mutableStateOf<FileItem?>(null) }
    var showDelete by remember { mutableStateOf<FileItem?>(null) }
    var showShare by remember { mutableStateOf<FileItem?>(null) }
    var previewFile by remember { mutableStateOf<FileItem?>(null) }

    // SAF 选择文件上传
    val openDoc = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenDocument()
    ) { uri: Uri? ->
        if (uri != null) {
            try {
                context.contentResolver.takePersistableUriPermission(
                    uri, Intent.FLAG_GRANT_READ_URI_PERMISSION
                )
            } catch (_: Exception) {
                // 部分 Provider 不支持持久授权，忽略
            }
            val name = queryDisplayName(context, uri) ?: "未命名文件"
            val size = querySize(context, uri)
            com.happycloud.android.ui.upload.UploadService.start(
                context, uri, name, size, viewModel.currentParentId,
            )
        }
    }

    LaunchedEffect(state.snackbar) {
        state.snackbar?.let {
            snackbarHostState.showSnackbar(it)
            viewModel.consumeSnackbar()
        }
    }

    Scaffold(
        modifier = modifier,
        snackbarHost = { SnackbarHost(snackbarHostState) },
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            Surface(
                modifier = Modifier.statusBarsPadding(),
                color = MaterialTheme.colorScheme.surfaceContainerLow,
            ) {
                Column {
                    // 来源切换：全部 / 最近 / 收藏
                    SourceSwitcher(
                        source = state.source,
                        onSwitch = viewModel::switchSource,
                    )
                    // 面包屑 + 新建文件夹 + 视图切换
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(start = 8.dp, end = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        LazyRow(modifier = Modifier.weight(1f)) {
                            items(state.breadcrumb.size) { index ->
                                val crumb = state.breadcrumb[index]
                                TextButton(onClick = { viewModel.navigateTo(index) }) {
                                    Text(
                                        text = crumb.name,
                                        style = MaterialTheme.typography.titleMedium,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis,
                                    )
                                }
                                if (index < state.breadcrumb.lastIndex) {
                                    Text(
                                        text = "/",
                                        color = MaterialTheme.colorScheme.outline,
                                        style = MaterialTheme.typography.titleMedium,
                                    )
                                }
                            }
                        }
                        IconButton(onClick = { showNewFolder = true }) {
                            Icon(
                                Icons.Filled.CreateNewFolder,
                                contentDescription = "新建文件夹",
                                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        ViewModeSwitch(gridMode = state.gridMode, onToggle = viewModel::toggleGrid)
                    }
                    // 空间用量条
                    QuotaBar(used = state.quotaUsed, max = state.quotaMax)
                    // P2P 连接状态
                    P2PStatusChip(mode = p2pMode, status = p2pStatus)
                    // 存储概览卡片（全部文件视图展示分类占用）
                    if (state.source == FileSource.ALL && state.overview != null) {
                        OverviewCard(overview = state.overview!!)
                    }
                    // hairline 分隔，与内容区隔离
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                }
            }
        },
        floatingActionButton = {
            ExtendedFloatingActionButton(
                onClick = { openDoc.launch(arrayOf("*/*")) },
                icon = { Icon(Icons.Filled.Upload, contentDescription = null) },
                text = { Text("上传") },
                shape = com.happycloud.android.ui.theme.FabShape,
            )
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            PullToRefreshBox(
                isRefreshing = state.refreshing,
                onRefresh = viewModel::refresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                when {
                    state.loading && state.items.isEmpty() -> {
                        LoadingPlaceholder()
                    }
                    state.error != null && state.items.isEmpty() -> {
                        ErrorPlaceholder(
                            message = state.error.orEmpty(),
                            onRetry = viewModel::refresh,
                        )
                    }
                    state.items.isEmpty() -> {
                        EmptyPlaceholder(
                            source = state.source,
                            onUpload = { openDoc.launch(arrayOf("*/*")) },
                        )
                    }
                    state.gridMode -> {
                        LazyVerticalGrid(
                            columns = GridCells.Adaptive(120.dp),
                            contentPadding = PaddingValues(16.dp),
                            horizontalArrangement = Arrangement.spacedBy(12.dp),
                            verticalArrangement = Arrangement.spacedBy(12.dp),
                        ) {
                            items(state.items) { file ->
                                FileGridCard(
                                    file = file,
                                    isFavorite = state.favoriteIds.contains(file.id),
                                    onToggleFavorite = { viewModel.toggleFavorite(file) },
                                    onClick = { onFileClick(file, viewModel) { previewFile = it } },
                                    onLongClick = { menuTarget = file },
                                )
                            }
                        }
                    }
                    else -> {
                        LazyColumn(
                            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                            verticalArrangement = Arrangement.spacedBy(10.dp),
                        ) {
                            items(state.items) { file ->
                                FileListRow(
                                    file = file,
                                    isFavorite = state.favoriteIds.contains(file.id),
                                    onToggleFavorite = { viewModel.toggleFavorite(file) },
                                    onClick = { onFileClick(file, viewModel) { previewFile = it } },
                                    onLongClick = { menuTarget = file },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    // 长按菜单
    menuTarget?.let { target ->
        DropdownMenu(expanded = true, onDismissRequest = { menuTarget = null }) {
            if (PreviewUtil.canInlinePreview(target.name) || PreviewUtil.canPlay(target.name)) {
                DropdownMenuItem(
                    text = { Text(if (PreviewUtil.canPlay(target.name)) "播放" else "预览") },
                    onClick = {
                        menuTarget = null
                        onFileClick(target, viewModel) { previewFile = it }
                    },
                )
            }
            DropdownMenuItem(
                text = { Text("重命名") },
                onClick = {
                    menuTarget = null
                    showRename = target
                },
            )
            DropdownMenuItem(
                text = { Text("移动") },
                onClick = {
                    menuTarget = null
                    showMove = target
                },
            )
            DropdownMenuItem(
                text = { Text("分享") },
                onClick = {
                    menuTarget = null
                    showShare = target
                },
            )
            DropdownMenuItem(
                text = { Text("删除") },
                onClick = {
                    menuTarget = null
                    showDelete = target
                },
            )
        }
    }

    if (showNewFolder) {
        NewFolderDialog(
            onConfirm = { name -> viewModel.mkdir(name) { showNewFolder = false } },
            onDismiss = { showNewFolder = false },
        )
    }
    showRename?.let { target ->
        RenameDialog(
            file = target,
            onConfirm = { name -> viewModel.rename(target, name) { showRename = null } },
            onDismiss = { showRename = null },
        )
    }
    showMove?.let { target ->
        MoveDialog(
            container = container,
            file = target,
            onConfirm = { parentId -> viewModel.move(target, parentId) { showMove = null } },
            onDismiss = { showMove = null },
        )
    }
    showDelete?.let { target ->
        DeleteDialog(
            file = target,
            onConfirm = { viewModel.delete(target) { showDelete = null } },
            onDismiss = { showDelete = null },
        )
    }
    showShare?.let { target ->
        ShareDialog(
            container = container,
            file = target,
            onDismiss = { showShare = null },
        )
    }
    previewFile?.let { target ->
        PreviewDialog(
            container = container,
            file = target,
            onDismiss = { previewFile = null },
        )
    }
}

/** 点击条目：文件夹进入；图片/文本内联预览；音视频系统播放；其余下载 */
private fun onFileClick(
    file: FileItem,
    viewModel: FilesViewModel,
    onPreview: (FileItem) -> Unit,
) {
    when {
        file.isFolder -> viewModel.enterFolder(file)
        PreviewUtil.canInlinePreview(file.name) -> onPreview(file)
        PreviewUtil.canPlay(file.name) -> viewModel.play(file)
        else -> viewModel.download(file)
    }
}

// ---------- 顶部组件 ----------

@Composable
private fun SourceSwitcher(source: FileSource, onSwitch: (FileSource) -> Unit) {
    val labels = listOf(
        FileSource.ALL to "全部",
        FileSource.RECENT to "最近",
        FileSource.FAVORITE to "收藏",
    )
    SingleChoiceSegmentedButtonRow(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        val colors = SegmentedButtonDefaults.colors(
            activeContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
            activeContentColor = MaterialTheme.colorScheme.primary,
            inactiveContainerColor = Color.Transparent,
            inactiveContentColor = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        labels.forEachIndexed { index, (value, label) ->
            SegmentedButton(
                selected = source == value,
                onClick = { onSwitch(value) },
                shape = SegmentedButtonDefaults.itemShape(index = index, count = labels.size),
                colors = colors,
            ) {
                Text(label, style = MaterialTheme.typography.labelLarge)
            }
        }
    }
}

@Composable
private fun ViewModeSwitch(gridMode: Boolean, onToggle: (Boolean) -> Unit) {
    val colors = SegmentedButtonDefaults.colors(
        activeContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
        activeContentColor = MaterialTheme.colorScheme.onSurface,
        inactiveContainerColor = Color.Transparent,
        inactiveContentColor = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    SingleChoiceSegmentedButtonRow(modifier = Modifier.padding(end = 8.dp)) {
        SegmentedButton(
            selected = gridMode,
            onClick = { onToggle(true) },
            shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
            colors = colors,
            icon = { Icon(Icons.Filled.Dashboard, contentDescription = null) },
        ) {}
        SegmentedButton(
            selected = !gridMode,
            onClick = { onToggle(false) },
            shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
            colors = colors,
            icon = { Icon(Icons.Filled.ViewList, contentDescription = null) },
        ) {}
    }
}

@Composable
private fun QuotaBar(used: Long, max: Long) {
    val fraction = if (max <= 0) 0f else (used.toFloat() / max.toFloat()).coerceIn(0f, 1f)
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp),
        shape = MaterialTheme.shapes.medium,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Text(
                    text = "空间用量",
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = "${FormatUtil.bytes(used)} / ${FormatUtil.bytes(max)}",
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.height(6.dp))
            LinearProgressIndicator(
                progress = { fraction },
                modifier = Modifier
                    .fillMaxWidth()
                    .height(6.dp),
                color = MaterialTheme.colorScheme.primary,
                trackColor = MaterialTheme.colorScheme.surfaceContainerHighest,
                strokeCap = androidx.compose.ui.graphics.StrokeCap.Round,
            )
        }
    }
}

/** 存储概览：按分类展示占用条 + 图例 */
@Composable
private fun OverviewCard(overview: StorageOverview) {
    val cat = overview.categories
    val total = cat.total
    val segments = listOf(
        "图片" to cat.image to CategoryColor.image,
        "视频" to cat.video to CategoryColor.video,
        "音频" to cat.audio to CategoryColor.audio,
        "文档" to cat.doc to CategoryColor.doc,
        "其他" to cat.other to CategoryColor.other,
    )
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(16.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = "存储概览",
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = FormatUtil.bytes(overview.quotaUsed),
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onSurface,
                )
            }
            Spacer(Modifier.height(10.dp))
            // 分段用量条
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(10.dp),
            ) {
                if (total <= 0) {
                    Box(
                        Modifier
                            .weight(1f)
                            .fillMaxHeight()
                            .background(
                                MaterialTheme.colorScheme.surfaceContainerHighest,
                                RoundedCornerShape(5.dp),
                            ),
                    )
                } else {
                    segments.forEach { (pair, color) ->
                        val (_, value) = pair
                        val frac = value.toFloat() / total
                        if (frac <= 0f) return@forEach
                        Box(
                            Modifier
                                .weight(frac)
                                .fillMaxHeight()
                                .padding(horizontal = 1.dp)
                                .background(color, RoundedCornerShape(5.dp)),
                        )
                    }
                }
            }
            Spacer(Modifier.height(12.dp))
            // 图例
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                segments.forEach { (pair, color) ->
                    val (label, value) = pair
                    Column(
                        modifier = Modifier.weight(1f),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Box(
                            Modifier
                                .size(8.dp)
                                .background(color, RoundedCornerShape(4.dp)),
                        )
                        Spacer(Modifier.height(4.dp))
                        Text(
                            text = label,
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Text(
                            text = FormatUtil.bytes(value),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurface,
                        )
                    }
                }
            }
        }
    }
}

/** 分类颜色（青绿色相明度阶梯 + 灰阶，克制不喧宾夺主） */
private object CategoryColor {
    val image = Color(0xFF11695F)   // 主调：青绿
    val video = Color(0xFF4C8C84)   // 青绿浅一档
    val audio = Color(0xFF7A9C97)   // 青绿灰化
    val doc = Color(0xFFB0B3AE)     // 石墨灰
    val other = Color(0xFFD6D4D1)   // 浅石墨
}

// ---------- 列表/网格条目 ----------

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun FileGridCard(
    file: FileItem,
    isFavorite: Boolean,
    onToggleFavorite: () -> Unit,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .combinedClickable(onClick = onClick, onLongClick = onLongClick),
        shape = MaterialTheme.shapes.medium,
        color = MaterialTheme.colorScheme.surfaceContainerLowest,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            horizontalAlignment = Alignment.Start,
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(
                    imageVector = if (file.isFolder) Icons.Filled.Folder else Icons.Filled.InsertDriveFile,
                    contentDescription = null,
                    modifier = Modifier.size(30.dp),
                    tint = if (file.isFolder) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )
                if (!file.isFolder) {
                    IconButton(onClick = onToggleFavorite, modifier = Modifier.size(28.dp)) {
                        Icon(
                            imageVector = if (isFavorite) Icons.Filled.Star else Icons.Filled.StarBorder,
                            contentDescription = if (isFavorite) "取消收藏" else "收藏",
                            modifier = Modifier.size(20.dp),
                            tint = if (isFavorite) FAVORITE_ACTIVE else MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                } else {
                    Spacer(Modifier.size(28.dp))
                }
            }
            Spacer(Modifier.height(8.dp))
            Text(
                text = file.name,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(2.dp))
            Text(
                text = if (file.isFolder) "文件夹" else FormatUtil.bytes(file.size),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun FileListRow(
    file: FileItem,
    isFavorite: Boolean,
    onToggleFavorite: () -> Unit,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .combinedClickable(onClick = onClick, onLongClick = onLongClick),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLowest,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(
                imageVector = if (file.isFolder) Icons.Filled.Folder else Icons.Filled.InsertDriveFile,
                contentDescription = null,
                modifier = Modifier.size(28.dp),
                tint = if (file.isFolder) {
                    MaterialTheme.colorScheme.primary
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
            )
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    text = file.name,
                    style = MaterialTheme.typography.titleMedium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    text = if (file.isFolder) {
                        "文件夹 · ${FormatUtil.date(file.createdAt)}"
                    } else {
                        "${FormatUtil.bytes(file.size)} · ${FormatUtil.date(file.createdAt)}"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (!file.isFolder) {
                IconButton(onClick = onToggleFavorite) {
                    Icon(
                        imageVector = if (isFavorite) Icons.Filled.Star else Icons.Filled.StarBorder,
                        contentDescription = if (isFavorite) "取消收藏" else "收藏",
                        modifier = Modifier.size(22.dp),
                        tint = if (isFavorite) FAVORITE_ACTIVE else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            IconButton(onClick = onLongClick) {
                Icon(
                    Icons.Filled.MoreVert,
                    contentDescription = "更多操作",
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

// ---------- 占位状态 ----------

@Composable
private fun LoadingPlaceholder() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator()
    }
}

@Composable
private fun ErrorPlaceholder(message: String, onRetry: () -> Unit) {
    ErrorState(message = message, onRetry = onRetry)
}

@Composable
private fun EmptyPlaceholder(source: FileSource, onUpload: () -> Unit) {
    EmptyState(
        icon = Icons.Filled.FolderOpen,
        title = when (source) {
            FileSource.ALL -> "这里空空如也"
            FileSource.RECENT -> "暂无最近文件"
            FileSource.FAVORITE -> "还没有收藏"
        },
        subtitle = when (source) {
            FileSource.ALL -> "上传第一个文件，或新建文件夹开始整理"
            FileSource.RECENT -> "访问过的文件会出现在这里"
            FileSource.FAVORITE -> "点击文件上的星标即可收藏"
        },
        actionLabel = if (source == FileSource.ALL) "上传文件" else null,
        onAction = if (source == FileSource.ALL) onUpload else null,
    )
}

// ---------- 工具 ----------

/** P2P 打洞连接状态标签 */
@Composable
private fun P2PStatusChip(
    mode: com.happycloud.android.data.p2p.ConnMode,
    status: String,
) {
    val (dot, label, tint) = when (mode) {
        com.happycloud.android.data.p2p.ConnMode.P2P -> Triple("●", "P2P 直连", Color(0xFF2E9E5B))
        com.happycloud.android.data.p2p.ConnMode.HTTP -> Triple("○", status, MaterialTheme.colorScheme.outline)
    }
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        shape = RoundedCornerShape(12.dp),
        contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = dot,
                color = tint,
                style = MaterialTheme.typography.labelMedium,
            )
            Spacer(Modifier.width(6.dp))
            Text(
                text = label,
                style = MaterialTheme.typography.labelMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

private fun queryDisplayName(context: android.content.Context, uri: Uri): String? {
    return try {
        context.contentResolver.query(
            uri, arrayOf(android.provider.OpenableColumns.DISPLAY_NAME), null, null, null
        )?.use { c ->
            if (c.moveToFirst()) c.getString(0) else null
        }
    } catch (_: Exception) {
        null
    }
}

private fun querySize(context: android.content.Context, uri: Uri): Long {
    return try {
        context.contentResolver.query(
            uri, arrayOf(android.provider.OpenableColumns.SIZE), null, null, null
        )?.use { c ->
            if (c.moveToFirst() && !c.isNull(0)) c.getLong(0) else 0L
        } ?: 0L
    } catch (_: Exception) {
        0L
    }
}
