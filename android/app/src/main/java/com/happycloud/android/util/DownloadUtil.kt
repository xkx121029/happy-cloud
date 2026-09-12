package com.happycloud.android.util

import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.webkit.MimeTypeMap
import androidx.core.content.FileProvider
import com.happycloud.android.data.ApiService
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/** 下载工具：OkHttp 拉流，保存到系统「下载/HappyCloud」目录 */
object DownloadUtil {

    /** 音视频缓存到应用缓存目录，交给系统播放器播放 */
    suspend fun play(context: Context, api: ApiService, fileId: Long, name: String) =
        withContext(Dispatchers.IO) {
            val body = api.download(fileId)
            val mime = MimeTypeMap.getSingleton()
                .getMimeTypeFromExtension(name.substringAfterLast('.', "").lowercase())
                ?: "application/octet-stream"
            val cacheDir = File(context.cacheDir, "media").apply { mkdirs() }
            val out = File(cacheDir, System.currentTimeMillis().toString() + "_" + name)
            body.byteStream().use { input -> out.outputStream().use { it.copyFrom(input) } }
            val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", out)
            val intent = Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, mime)
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            context.startActivity(intent)
            "正在调用系统播放器播放「$name」"
        }

    private fun java.io.OutputStream.copyFrom(input: java.io.InputStream) {
        val buf = ByteArray(64 * 1024)
        while (true) {
            val n = input.read(buf)
            if (n < 0) break
            write(buf, 0, n)
        }
    }

    suspend fun download(context: Context, api: ApiService, fileId: Long, name: String): String =
        withContext(Dispatchers.IO) {
            val body = api.download(fileId)
            val mime = MimeTypeMap.getSingleton()
                .getMimeTypeFromExtension(name.substringAfterLast('.', "").lowercase())
                ?: "application/octet-stream"

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                // Android 10+：MediaStore，免存储权限
                val values = ContentValues().apply {
                    put(MediaStore.MediaColumns.DISPLAY_NAME, name)
                    put(MediaStore.MediaColumns.MIME_TYPE, mime)
                    put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS + "/HappyCloud")
                    put(MediaStore.MediaColumns.IS_PENDING, 1)
                }
                val resolver = context.contentResolver
                val uri = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
                    ?: throw IllegalStateException("无法创建下载文件")
                try {
                    resolver.openOutputStream(uri)?.use { out ->
                        body.byteStream().use { input -> input.copyTo(out) }
                    } ?: throw IllegalStateException("无法写入下载文件")
                    values.clear()
                    values.put(MediaStore.MediaColumns.IS_PENDING, 0)
                    resolver.update(uri, values, null, null)
                    "已保存到 下载/HappyCloud/$name"
                } catch (e: Exception) {
                    resolver.delete(uri, null, null)
                    throw e
                }
            } else {
                // Android 9 及以下：公共下载目录（已声明 maxSdkVersion=28 的写权限）
                val dir = File(
                    Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS),
                    "HappyCloud",
                )
                if (!dir.exists()) dir.mkdirs()
                val out = File(dir, name)
                body.byteStream().use { input ->
                    out.outputStream().use { output -> input.copyTo(output) }
                }
                "已保存到 下载/HappyCloud/$name"
            }
        }
}
