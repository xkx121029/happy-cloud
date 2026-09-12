package com.happycloud.android.util

import com.happycloud.android.data.Network

/** 在线预览：按扩展名判断类型，并构造带鉴权的预览地址 */
object PreviewUtil {

    private val IMAGE = setOf("png", "jpg", "jpeg", "gif", "webp", "bmp")
    private val VIDEO = setOf("mp4", "m4v", "webm", "mkv", "mov", "avi", "3gp", "3gpp")
    private val AUDIO = setOf("mp3", "wav", "aac", "flac", "m4a", "ogg", "opus", "amr")
    private val TEXT = setOf(
        "txt", "md", "markdown", "log", "json", "xml", "yml", "yaml", "csv", "ini", "cfg", "conf", "sql",
        "css", "js", "ts", "html", "htm", "kt", "kts", "java", "py", "go", "c", "cpp", "cc", "h", "hpp",
        "sh", "bat", "ps1", "gradle", "properties", "toml", "lock",
    )

    private fun ext(name: String): String =
        name.substringAfterLast('.', "").lowercase()

    fun isImage(name: String) = ext(name) in IMAGE
    fun isVideo(name: String) = ext(name) in VIDEO
    fun isAudio(name: String) = ext(name) in AUDIO
    fun isText(name: String) = ext(name) in TEXT

    /** 支持在应用内联预览（图片/文本） */
    fun canInlinePreview(name: String) = isImage(name) || isText(name)

    /** 支持交给系统播放器播放（视频/音频） */
    fun canPlay(name: String) = isVideo(name) || isAudio(name)

    /** 预览流式地址（需在请求头携带 Authorization） */
    fun previewUrl(fileId: Long): String =
        Network.BASE_URL.trimEnd('/') + "/api/files/preview?file_id=$fileId"
}
