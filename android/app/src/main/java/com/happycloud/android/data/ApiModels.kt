package com.happycloud.android.data

import com.google.gson.annotations.SerializedName

/** 统一响应包装 {code, message, data} */
data class ApiResponse<T>(
    val code: Int,
    val message: String,
    val data: T? = null,
)

/** 用户 */
data class User(
    val id: Long,
    val username: String,
    val email: String? = null,
    val role: Int = 0,
    @SerializedName("quota_max") val quotaMax: Long = 0,
    @SerializedName("quota_used") val quotaUsed: Long = 0,
    val status: Int = 0,
    @SerializedName("created_at") val createdAt: String? = null,
)

/** 登录/注册返回 */
data class AuthData(val token: String, val user: User)

/** 文件/文件夹：type 0 文件夹 / 1 文件 */
data class FileItem(
    val id: Long,
    @SerializedName("parent_id") val parentId: Long,
    val name: String,
    val type: Int,
    val size: Long = 0,
    val hash: String? = null,
    @SerializedName("created_at") val createdAt: String? = null,
) {
    val isFolder: Boolean get() = type == 0
}

/** 文件列表（后端返回 items 字段） */
data class FileListData(
    val total: Int = 0,
    val page: Int = 1,
    @SerializedName("page_size") val pageSize: Int = 100,
    val items: List<FileItem> = emptyList(),
)

/** 空间用量 */
data class Quota(
    @SerializedName("quota_max") val quotaMax: Long = 0,
    @SerializedName("quota_used") val quotaUsed: Long = 0,
) {
    val fraction: Float
        get() = if (quotaMax <= 0) 0f else (quotaUsed.toFloat() / quotaMax.toFloat()).coerceIn(0f, 1f)
}

/** 秒传校验结果 */
data class HashCheck(
    val exists: Boolean = false,
    @SerializedName("file_id") val fileId: Long = 0,
)

/** 分享创建结果 */
data class ShareResult(val token: String, val url: String)
