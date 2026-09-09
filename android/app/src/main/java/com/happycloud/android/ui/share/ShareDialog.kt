package com.happycloud.android.ui.share

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
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
import com.happycloud.android.AppContainer
import com.happycloud.android.data.ApiException
import com.happycloud.android.data.FileItem
import com.happycloud.android.data.Network
import com.happycloud.android.data.ShareResult
import com.happycloud.android.data.safeApi
import com.happycloud.android.ui.theme.ButtonShape
import kotlinx.coroutines.launch

/** 分享弹窗：创建链接（可选密码）、复制链接 */
@Composable
fun ShareDialog(
    container: AppContainer,
    file: FileItem,
    onDismiss: () -> Unit,
    snackbarHostState: SnackbarHostState? = null,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var usePassword by remember { mutableStateOf(false) }
    var password by remember { mutableStateOf("") }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var result by remember { mutableStateOf<ShareResult?>(null) }

    val fullLink = result?.let { Network.BASE_URL.trimEnd('/') + it.url }

    fun copyLink() {
        fullLink?.let {
            val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            cm.setPrimaryClip(ClipData.newPlainText("分享链接", it))
            scope.launch {
                snackbarHostState?.showSnackbar("链接已复制", duration = SnackbarDuration.Short)
            }
        }
    }

    AlertDialog(
        onDismissRequest = { if (!loading) onDismiss() },
        title = { Text("分享「${file.name}」", style = MaterialTheme.typography.titleLarge) },
        text = {
            Column {
                if (result == null) {
                    // 设置密码（可选）
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(
                            checked = usePassword,
                            onCheckedChange = { usePassword = it; error = null },
                        )
                        Spacer(Modifier.width(6.dp))
                        Text("设置提取密码", style = MaterialTheme.typography.bodyLarge)
                    }
                    if (usePassword) {
                        OutlinedTextField(
                            value = password,
                            onValueChange = { password = it; error = null },
                            label = { Text("密码") },
                            singleLine = true,
                            shape = ButtonShape,
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                    Text(
                        text = "生成的链接可直接分享给他人，文件夹与文件均可分享",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 10.dp),
                    )
                } else {
                    // 展示链接 + 复制
                    OutlinedTextField(
                        value = fullLink.orEmpty(),
                        onValueChange = {},
                        readOnly = true,
                        label = { Text("分享链接") },
                        shape = ButtonShape,
                        modifier = Modifier.fillMaxWidth(),
                        trailingIcon = {
                            TextButton(onClick = ::copyLink) {
                                Icon(
                                    Icons.Filled.ContentCopy,
                                    contentDescription = "复制",
                                    modifier = Modifier.size(18.dp),
                                )
                            }
                        },
                    )
                    Text(
                        text = if (usePassword) "已设置密码，对方访问时需输入" else "无密码，任何人可访问",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 10.dp),
                    )
                }
                if (error != null) {
                    Text(
                        text = error.orEmpty(),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
            }
        },
        confirmButton = {
            if (result == null) {
                Button(
                    onClick = {
                        if (usePassword && password.isBlank()) {
                            error = "请输入提取密码"
                            return@Button
                        }
                        scope.launch {
                            loading = true
                            error = null
                            try {
                                val body = buildMap {
                                    put("file_id", file.id)
                                    if (usePassword) put("password", password)
                                }
                                result = safeApi { container.api.createShare(body) }
                            } catch (e: ApiException) {
                                error = e.message
                            } finally {
                                loading = false
                            }
                        }
                    },
                    enabled = !loading,
                    shape = ButtonShape,
                ) {
                    if (loading) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(20.dp),
                            strokeWidth = 2.dp,
                            color = MaterialTheme.colorScheme.onPrimary,
                        )
                    } else {
                        Text("创建链接")
                    }
                }
            } else {
                Button(onClick = onDismiss, shape = ButtonShape) {
                    Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("完成")
                }
            }
        },
        dismissButton = {
            if (result == null) {
                TextButton(onClick = onDismiss, enabled = !loading) { Text("取消") }
            } else {
                TextButton(onClick = ::copyLink) { Text("复制链接") }
            }
        },
        shape = MaterialTheme.shapes.extraLarge,
    )
}
