package com.happycloud.android.ui.files

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.happycloud.android.AppContainer
import com.happycloud.android.data.FileItem
import com.happycloud.android.data.ApiException
import com.happycloud.android.data.safeApi
import com.happycloud.android.ui.theme.ButtonShape
import kotlinx.coroutines.launch

/** 新建文件夹 */
@Composable
fun NewFolderDialog(
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("新建文件夹", style = MaterialTheme.typography.titleLarge) },
        text = {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text("文件夹名称") },
                singleLine = true,
                shape = ButtonShape,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        confirmButton = {
            Button(
                onClick = { if (name.isNotBlank()) onConfirm(name) },
                enabled = name.isNotBlank(),
                shape = ButtonShape,
            ) { Text("创建") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("取消") }
        },
        shape = MaterialTheme.shapes.extraLarge,
    )
}

/** 重命名 */
@Composable
fun RenameDialog(
    file: FileItem,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by remember(file.id) { mutableStateOf(file.name) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("重命名", style = MaterialTheme.typography.titleLarge) },
        text = {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text("新名称") },
                singleLine = true,
                shape = ButtonShape,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        confirmButton = {
            Button(
                onClick = { if (name.isNotBlank()) onConfirm(name) },
                enabled = name.isNotBlank() && name != file.name,
                shape = ButtonShape,
            ) { Text("确定") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("取消") }
        },
        shape = MaterialTheme.shapes.extraLarge,
    )
}

/** 删除确认 */
@Composable
fun DeleteDialog(
    file: FileItem,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("删除确认", style = MaterialTheme.typography.titleLarge) },
        text = {
            Text(
                text = if (file.isFolder) {
                    "删除文件夹「${file.name}」及其全部内容？此操作不可恢复。"
                } else {
                    "确定删除文件「${file.name}」？此操作不可恢复。"
                },
                style = MaterialTheme.typography.bodyLarge,
            )
        },
        confirmButton = {
            Button(onClick = onConfirm, shape = ButtonShape) { Text("删除") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("取消") }
        },
        shape = MaterialTheme.shapes.extraLarge,
    )
}

/** 移动：选择目标文件夹（从根目录逐层进入） */
@Composable
fun MoveDialog(
    container: AppContainer,
    file: FileItem,
    onConfirm: (Long) -> Unit,
    onDismiss: () -> Unit,
) {
    var currentFolderId by remember { mutableStateOf(0L) }
    var folders by remember { mutableStateOf<List<FileItem>>(emptyList()) }
    var path by remember { mutableStateOf<List<FileItem>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(currentFolderId) {
        loading = true
        error = null
        try {
            val data = safeApi { container.api.listFiles(currentFolderId) }
            folders = data.items.filter { it.isFolder }
        } catch (e: ApiException) {
            error = e.message
            folders = emptyList()
        } finally {
            loading = false
        }
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("移动「${file.name}」到", style = MaterialTheme.typography.titleLarge) },
        text = {
            Column(Modifier.height(320.dp)) {
                // 路径导航
                Row(verticalAlignment = Alignment.CenterVertically) {
                    TextButton(
                        onClick = {
                            if (path.isEmpty()) {
                                currentFolderId = 0L
                            } else {
                                val last = path.last()
                                path = path.dropLast(1)
                                currentFolderId = last.parentId
                            }
                        },
                        enabled = true,
                    ) {
                        Icon(
                            Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "上级",
                            modifier = Modifier.size(18.dp),
                        )
                        Spacer(Modifier.size(4.dp))
                        Text(if (path.isEmpty()) "根目录" else "上级")
                    }
                    Text(
                        text = if (currentFolderId == 0L) "根目录" else path.lastOrNull()?.name ?: "",
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f),
                    )
                }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)

                when {
                    loading -> {
                        Spacer(Modifier.height(24.dp))
                        CircularProgressIndicator(
                            modifier = Modifier
                                .align(Alignment.CenterHorizontally)
                                .size(28.dp),
                            strokeWidth = 3.dp,
                        )
                    }
                    error != null -> {
                        Spacer(Modifier.height(16.dp))
                        Text(
                            text = error.orEmpty(),
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                    folders.isEmpty() -> {
                        Spacer(Modifier.height(24.dp))
                        Text(
                            text = "当前目录没有子文件夹",
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.align(Alignment.CenterHorizontally),
                        )
                    }
                    else -> {
                        LazyColumn(
                            verticalArrangement = Arrangement.spacedBy(2.dp),
                            modifier = Modifier.weight(1f),
                        ) {
                            items(folders, key = { it.id }) { folder ->
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .padding(vertical = 8.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                ) {
                                    Icon(
                                        Icons.Filled.Folder,
                                        contentDescription = null,
                                        tint = MaterialTheme.colorScheme.primary,
                                    )
                                    Spacer(Modifier.size(10.dp))
                                    Text(
                                        text = folder.name,
                                        style = MaterialTheme.typography.bodyLarge,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis,
                                        modifier = Modifier.weight(1f),
                                    )
                                    TextButton(onClick = {
                                        path = path + folder
                                        currentFolderId = folder.id
                                    }) { Text("进入") }
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            Button(
                onClick = { onConfirm(currentFolderId) },
                shape = ButtonShape,
            ) { Text("移动到此处") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("取消") }
        },
        shape = MaterialTheme.shapes.extraLarge,
    )
}
