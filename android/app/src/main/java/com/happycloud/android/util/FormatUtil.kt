package com.happycloud.android.util

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/** 通用格式化 */
object FormatUtil {

    private val dateFormat = SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.getDefault())

    fun bytes(size: Long): String {
        if (size < 1024) return "$size B"
        val kb = size / 1024.0
        if (kb < 1024) return String.format(Locale.getDefault(), "%.1f KB", kb)
        val mb = kb / 1024.0
        if (mb < 1024) return String.format(Locale.getDefault(), "%.1f MB", mb)
        val gb = mb / 1024.0
        return String.format(Locale.getDefault(), "%.2f GB", gb)
    }

    fun date(iso: String?): String {
        if (iso.isNullOrBlank()) return ""
        return try {
            dateFormat.format(Date(iso))
        } catch (_: Exception) {
            iso
        }
    }
}
