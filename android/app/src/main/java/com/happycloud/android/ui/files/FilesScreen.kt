package com.happycloud.android.ui.files

import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CreateNewFolder
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Upload
import androidx.compose.material.icons.filled.ViewList
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.happycloud.android.AppContainer
import com.happycloud.android.data.FileItem
import com.happycloud.android.ui.share.ShareDialog
import com.happycloud.android.ui.theme.ButtonShape
import com.happycloud.android.util.FormatUtil

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

    // 菜单与对话框状态
    var menuTarget by remember { mutableStateOf<FileItem?>(null) }
    var showNewFolder by remember { mutableStateOf(false) }
    var showRename by remember { mutableStateOf<FileItem?>(null) }
    var showMove by remember { mutableStateOf<FileItem?>(null) }
    var showDelete by remember { mutableStateOf<FileItem?>(null) }
    var showShare by remember { mutableStateOf<FileItem?>(null) }

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
            Column {
                // 面包屑 + 视图切换 + 新建文件夹
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(start = 8.dp, end = 4.dp, top = 4.dp),
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
                            items(state.items, key = { it.id }) { file ->
                                FileGridCard(
                                    file = file,
                                    onClick = { onFileClick(file, viewModel, openDoc) },
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
                            items(state.items, key = { it.id }) { file ->
                                FileListRow(
                                    file = file,
                                    onClick = { onFileClick(file, viewModel, openDoc) },
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
}

/** 点击条目：文件夹进入，文件下载 */
private fun onFileClick(
    file: FileItem,
    viewModel: FilesViewModel,
    openDoc: androidx.activity.compose.ManagedActivityResultLauncher<Array<String>, Uri?>,
) {
    if (file.isFolder) viewModel.enterFolder(file)
    else viewModel.download(file)
}

// ---------- 顶部组件 ----------

@Composable
private fun ViewModeSwitch(gridMode: Boolean, onToggle: (Boolean) -> Unit) {
    SingleChoiceSegmentedButtonRow(modifier = Modifier.padding(end = 8.dp)) {
        SegmentedButton(
            selected = gridMode,
            onClick = { onToggle(true) },
            shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
            icon = { Icon(Icons.Filled.Dashboard, contentDescription = null) },
        ) {}
        SegmentedButton(
            selected = !gridMode,
            onClick = { onToggle(false) },
            shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
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
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
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
                    .height(8.dp),
                color = MaterialTheme.colorScheme.primary,
                trackColor = MaterialTheme.colorScheme.surfaceContainerHighest,
                strokeCap = androidx.compose.ui.graphics.StrokeCap.Round,
            )
        }
    }
}

// ---------- 列表/网格条目 ----------

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun FileGridCard(
    file: FileItem,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .combinedClickable(onClick = onClick, onLongClick = onLongClick),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            horizontalAlignment = Alignment.Start,
        ) {
            Icon(
                imageVector = if (file.isFolder) Icons.Filled.Folder else Icons.Filled.InsertDriveFile,
                contentDescription = null,
                modifier = Modifier.size(34.dp),
                tint = if (file.isFolder) {
                    MaterialTheme.colorScheme.primary
                } else {
                    MaterialTheme.colorScheme.tertiary
                },
            )
            Spacer(Modifier.height(10.dp))
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
    onClick: () -> Unit,
    onLongClick: () -> Unit,
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .combinedClickable(onClick = onClick, onLongClick = onLongClick),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
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
                    MaterialTheme.colorScheme.tertiary
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
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
                text = message,
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.error,
            )
            Spacer(Modifier.height(12.dp))
            Button(onClick = onRetry, shape = ButtonShape) { Text("重试") }
        }
    }
}

@Composable
private fun EmptyPlaceholder(onUpload: () -> Unit) {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
                text = "这里空空如也",
                style = MaterialTheme.typography.titleLarge,
                color = MaterialTheme.colorScheme.onSurface,
            )
            Spacer(Modifier.height(6.dp))
            Text(
                text = "上传第一个文件，或新建文件夹开始整理",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(16.dp))
            Button(onClick = onUpload, shape = ButtonShape) {
                Icon(Icons.Filled.Upload, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text("上传文件")
            }
        }
    }
}

// ---------- 工具 ----------

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
