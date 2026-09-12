package com.happycloud.android.ui.preview

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.happycloud.android.AppContainer
import com.happycloud.android.data.FileItem
import com.happycloud.android.data.Network
import com.happycloud.android.ui.theme.ButtonShape
import com.happycloud.android.util.PreviewUtil
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** 在线预览弹窗：图片（带鉴权流式加载）/ 文本（拉流渲染） */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PreviewDialog(
    container: AppContainer,
    file: FileItem,
    onDismiss: () -> Unit,
) {
    val context = LocalContext.current
    val isImage = PreviewUtil.isImage(file.name)

    // 文本内容（仅文本类加载）
    var textContent by remember { mutableStateOf<String?>(null) }
    var textError by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(file.id) {
        if (!isImage) {
            try {
                val body = container.api.download(file.id)
                textContent = withContext(Dispatchers.IO) {
                    body.byteStream().bufferedReader().use { it.readText() }
                }
            } catch (e: Exception) {
                textError = "读取失败：${e.message}"
            }
        }
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Scaffold(
            containerColor = Color.Black,
            topBar = {
                TopAppBar(
                    title = {
                        Text(
                            text = file.name,
                            style = MaterialTheme.typography.titleMedium,
                            color = Color.White,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    },
                    navigationIcon = {
                        IconButton(onClick = onDismiss) {
                            Icon(
                                Icons.Filled.Close,
                                contentDescription = "关闭",
                                tint = Color.White,
                            )
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = Color.Transparent,
                        scrolledContainerColor = Color(0x99000000),
                    ),
                )
            },
        ) { padding ->
            Box(Modifier.fillMaxSize().padding(padding)) {
                if (isImage) {
                    val token = Network.tokenProvider?.invoke().orEmpty()
                    val headers = okhttp3.Headers.Builder()
                        .add("Authorization", "Bearer $token")
                        .build()
                    AsyncImage(
                        model = ImageRequest.Builder(context)
                            .data(PreviewUtil.previewUrl(file.id))
                            .headers(headers)
                            .crossfade(true)
                            .build(),
                        contentDescription = file.name,
                        modifier = Modifier
                            .fillMaxSize()
                            .background(Color.Black),
                        contentScale = ContentScale.Fit,
                    )
                } else {
                    Box(Modifier.fillMaxSize().background(Color(0xFF121212))) {
                        when {
                            textError != null -> {
                                Column(
                                    modifier = Modifier
                                        .fillMaxSize()
                                        .padding(24.dp),
                                    horizontalAlignment = Alignment.CenterHorizontally,
                                    verticalArrangement = androidx.compose.foundation.layout.Arrangement.Center,
                                ) {
                                    Text(
                                        text = textError.orEmpty(),
                                        color = MaterialTheme.colorScheme.error,
                                        style = MaterialTheme.typography.bodyLarge,
                                    )
                                    Spacer(Modifier.height(12.dp))
                                    androidx.compose.material3.TextButton(
                                        onClick = onDismiss,
                                        shape = ButtonShape,
                                    ) { Text("关闭", color = Color.White) }
                                }
                            }
                            textContent == null -> {
                                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                    CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                                }
                            }
                            else -> {
                                Text(
                                    text = textContent.orEmpty(),
                                    color = Color(0xFFE8E8E8),
                                    style = MaterialTheme.typography.bodyMedium,
                                    modifier = Modifier
                                        .fillMaxSize()
                                        .verticalScroll(rememberScrollState())
                                        .padding(horizontal = 20.dp, vertical = 16.dp),
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

