package com.happycloud.android.util

import android.content.ContentResolver
import android.content.Context
import android.net.Uri
import com.happycloud.android.data.ApiService
import com.happycloud.android.data.FileItem
import com.happycloud.android.data.safeApi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.BufferedInputStream
import java.security.MessageDigest

/**
 * 上传工具：SHA256 计算 + 直传/秒传/分片上传。
 * 阈值与大文件分片大小与后端契约一致：>100MB 走分片，每片 10MB。
 */
object UploadUtil {

    const val CHUNK_SIZE = 10L * 1024 * 1024
    const val LARGE_FILE_THRESHOLD = 100L * 1024 * 1024

    /** 计算整个文件的 SHA256（秒传校验用） */
    suspend fun sha256(context: Context, uri: Uri): String = withContext(Dispatchers.IO) {
        val digest = MessageDigest.getInstance("SHA-256")
        context.contentResolver.openInputStream(uri)?.use { input ->
            val buffer = ByteArray(8192)
            while (true) {
                val read = input.read(buffer)
                if (read < 0) break
                digest.update(buffer, 0, read)
            }
        } ?: throw IllegalStateException("无法读取文件")
        digest.digest().joinToString("") { "%02x".format(it) }
    }

    /**
     * 上传文件。
     * @param onProgress 进度回调 0..1（可能来自 IO 线程）
     * @return 上传成功后的文件记录；秒传命中时返回 null
     */
    suspend fun uploadFile(
        context: Context,
        api: ApiService,
        uri: Uri,
        name: String,
        size: Long,
        parentId: Long,
        onProgress: (Float) -> Unit,
    ): FileItem? = withContext(Dispatchers.IO) {
        if (size <= LARGE_FILE_THRESHOLD) {
            // ---------- 小文件直传 ----------
            val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
                ?: throw IllegalStateException("无法读取文件")
            onProgress(0.2f)
            val filePart = MultipartBody.Part.createFormData(
                "file", name,
                bytes.toRequestBody("application/octet-stream".toMediaType()),
            )
            val parentPart = parentId.toString().toRequestBody("text/plain".toMediaType())
            onProgress(0.6f)
            safeApi { api.upload(filePart, parentPart) }.also { onProgress(1f) }
        } else {
            // ---------- 大文件：秒传校验 → 分片 → 合并 ----------
            val hash = sha256(context, uri)
            val check = safeApi {
                api.uploadHash(
                    mapOf(
                        "name" to name,
                        "size" to size,
                        "hash" to hash,
                        "parent_id" to parentId,
                    )
                )
            }
            if (check.exists) {
                onProgress(1f)
                return@withContext null
            }
            val chunkTotal = ((size + CHUNK_SIZE - 1) / CHUNK_SIZE).toInt()
            val resolver = context.contentResolver
            for (i in 0 until chunkTotal) {
                val bytes = readChunk(resolver, uri, i.toLong() * CHUNK_SIZE, size)
                val part = MultipartBody.Part.createFormData(
                    "file", "chunk_$i",
                    bytes.toRequestBody("application/octet-stream".toMediaType()),
                )
                safeApi {
                    api.uploadChunk(
                        part,
                        hash.toRequestBody("text/plain".toMediaType()),
                        i.toString().toRequestBody("text/plain".toMediaType()),
                        chunkTotal.toString().toRequestBody("text/plain".toMediaType()),
                    )
                }
                onProgress(0.05f + 0.85f * (i + 1) / chunkTotal)
            }
            onProgress(0.95f)
            safeApi {
                api.uploadMerge(
                    mapOf(
                        "hash" to hash,
                        "name" to name,
                        "size" to size,
                        "parent_id" to parentId,
                        "chunk_total" to chunkTotal,
                    )
                )
            }.also { onProgress(1f) }
        }
    }

    /** 按偏移量读取一个分片（最后一片自动截断） */
    private fun readChunk(resolver: ContentResolver, uri: Uri, offset: Long, fileSize: Long): ByteArray {
        val len = minOf(CHUNK_SIZE, fileSize - offset).toInt()
        require(len > 0) { "分片长度为 0" }
        resolver.openInputStream(uri)?.use { raw ->
            val input = BufferedInputStream(raw)
            var skipped = 0L
            while (skipped < offset) {
                val s = input.skip(offset - skipped)
                if (s <= 0) break
                skipped += s
            }
            val bytes = ByteArray(len)
            var read = 0
            while (read < len) {
                val r = input.read(bytes, read, len - read)
                if (r < 0) break
                read += r
            }
            return bytes
        } ?: throw IllegalStateException("无法读取分片")
    }
}
