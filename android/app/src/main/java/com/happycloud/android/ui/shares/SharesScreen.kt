package com.happycloud.android.ui.shares

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
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
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.DeleteForever
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.happycloud.android.data.MyShareItem
import com.happycloud.android.data.Network
import com.happycloud.android.ui.components.EmptyState
import com.happycloud.android.ui.components.ErrorState
import com.happycloud.android.ui.theme.ButtonShape
import com.happycloud.android.ui.theme.DialogShape
import com.happycloud.android.util.FormatUtil
import kotlinx.coroutines.launch

/** 我的分享：列表 / 复制链接 / 设置有效期 / 取消分享 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SharesScreen(
    viewModel: SharesViewModel,
    onBack: () -> Unit,
) {
    val state by viewModel.state.collectAsState()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val snackbarHostState = remember { SnackbarHostState() }
    var expireTarget by remember { mutableStateOf<MyShareItem?>(null) }
    var cancelTarget by remember { mutableStateOf<MyShareItem?>(null) }

    fun copyLink(share: MyShareItem) {
        val link = Network.baseUrl.trimEnd('/') + "/share/" + share.token
        val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        cm.setPrimaryClip(ClipData.newPlainText("分享链接", link))
        scope.launch { snackbarHostState.showSnackbar("链接已复制") }
    }

    LaunchedEffect(state.snackbar) {
        state.snackbar?.let {
            snackbarHostState.showSnackbar(it)
            viewModel.consumeSnackbar()
        }
    }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbarHostState) },
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        text = if (state.items.isEmpty()) "我的分享" else "我的分享（${state.items.size}）",
                        style = MaterialTheme.typography.titleLarge,
                    )
                },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background,
                ),
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
                        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                            CircularProgressIndicator()
                        }
                    }
                    state.error != null && state.items.isEmpty() -> {
                        ErrorState(
                            message = state.error.orEmpty(),
                            onRetry = viewModel::refresh,
                        )
                    }
                    state.items.isEmpty() -> {
                        EmptyState(
                            icon = Icons.Filled.Link,
                            title = "还没有分享",
                            subtitle = "在文件列表长按文件即可创建分享链接",
                        )
                    }
                    else -> {
                        LazyColumn(
                            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                            verticalArrangement = Arrangement.spacedBy(10.dp),
                        ) {
                            items(state.items) { share ->
                                ShareRow(
                                    share = share,
                                    onCopy = { copyLink(share) },
                                    onExpire = { expireTarget = share },
                                    onCancel = { cancelTarget = share },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    expireTarget?.let { target ->
        ExpireDialog(
            share = target,
            onConfirm = { days -> viewModel.updateExpire(target, days) { expireTarget = null } },
            onDismiss = { expireTarget = null },
        )
    }
    cancelTarget?.let { target ->
        AlertDialog(
            onDismissRequest = { cancelTarget = null },
            title = { Text("取消分享", style = MaterialTheme.typography.titleLarge) },
            text = {
                Text(
                    text = "取消后，链接将立即失效，他人无法再访问「${target.fileName}」。确定取消？",
                    style = MaterialTheme.typography.bodyLarge,
                )
            },
            confirmButton = {
                Button(
                    onClick = { viewModel.cancel(target) { cancelTarget = null } },
                    shape = ButtonShape,
                    colors = androidx.compose.material3.ButtonDefaults.buttonColors(
                        containerColor = MaterialTheme.colorScheme.errorContainer,
                        contentColor = MaterialTheme.colorScheme.onErrorContainer,
                    ),
                ) { Text("取消分享") }
            },
            dismissButton = {
                TextButton(onClick = { cancelTarget = null }) { Text("返回") }
            },
            shape = DialogShape,
        )
    }
}

@Composable
private fun ShareRow(
    share: MyShareItem,
    onCopy: () -> Unit,
    onExpire: () -> Unit,
    onCancel: () -> Unit,
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLowest,
        border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    imageVector = if (share.fileType == 0) Icons.Filled.Folder else Icons.Filled.InsertDriveFile,
                    contentDescription = null,
                    modifier = Modifier.size(28.dp),
                    tint = if (share.fileType == 0) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        text = share.fileName.ifBlank { "（文件已删除）" },
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Spacer(Modifier.height(2.dp))
                    Text(
                        text = "${FormatUtil.bytes(share.fileSize)} · ${share.views} 次访问",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Spacer(Modifier.height(12.dp))
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        Icons.Filled.Schedule,
                        contentDescription = null,
                        modifier = Modifier.size(14.dp),
                        tint = if (share.isExpired) {
                            MaterialTheme.colorScheme.error
                        } else {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        },
                    )
                    Spacer(Modifier.width(4.dp))
                    Text(
                        text = when {
                            share.isExpired -> "已过期"
                            share.expireAt != null -> "有效期至 ${FormatUtil.date(share.expireAt)}"
                            else -> "永久有效"
                        },
                        style = MaterialTheme.typography.labelMedium,
                        color = if (share.isExpired) {
                            MaterialTheme.colorScheme.error
                        } else {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        },
                    )
                    if (share.passwordRequired) {
                        Spacer(Modifier.width(10.dp))
                        Icon(
                            Icons.Filled.Lock,
                            contentDescription = "有密码",
                            modifier = Modifier.size(14.dp),
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                Row {
                    TextButton(onClick = onCopy) {
                        Icon(
                            Icons.Filled.ContentCopy,
                            contentDescription = null,
                            modifier = Modifier.size(16.dp),
                        )
                        Spacer(Modifier.width(4.dp))
                        Text("复制")
                    }
                    TextButton(onClick = onExpire) { Text("有效期") }
                    IconButton(onClick = onCancel) {
                        Icon(
                            Icons.Filled.DeleteForever,
                            contentDescription = "取消分享",
                            tint = MaterialTheme.colorScheme.error,
                        )
                    }
                }
            }
        }
    }
}

/** 设置有效期：永久 / 1天 / 7天 / 30天 */
@Composable
private fun ExpireDialog(
    share: MyShareItem,
    onConfirm: (Int?) -> Unit,
    onDismiss: () -> Unit,
) {
    val options = listOf(
        "永久有效" to (null as Int?),
        "1 天" to 1,
        "7 天" to 7,
        "30 天" to 30,
    )
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("设置有效期", style = MaterialTheme.typography.titleLarge) },
        text = {
            Column {
                Text(
                    text = share.fileName,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(14.dp))
                options.forEachIndexed { index, (label, days) ->
                    Surface(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 3.dp),
                        shape = RoundedCornerShape(14.dp),
                        color = if (days == null && share.expireAt == null) {
                            MaterialTheme.colorScheme.primaryContainer
                        } else {
                            MaterialTheme.colorScheme.surfaceContainerHigh
                        },
                        onClick = { onConfirm(days) },
                    ) {
                        Box(Modifier.padding(horizontal = 16.dp, vertical = 12.dp)) {
                            Text(
                                text = label,
                                style = MaterialTheme.typography.bodyLarge,
                            )
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text("取消") }
        },
        shape = DialogShape,
    )
}
