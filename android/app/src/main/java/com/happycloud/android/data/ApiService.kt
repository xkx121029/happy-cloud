package com.happycloud.android.data

import okhttp3.MultipartBody
import okhttp3.RequestBody
import okhttp3.ResponseBody
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.HTTP
import retrofit2.http.Multipart
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Part
import retrofit2.http.Path
import retrofit2.http.Query
import retrofit2.http.Streaming

/** Happy-Cloud REST API（契约见 docs/api.md） */
interface ApiService {

    // ---------- 认证 ----------
    @POST("api/auth/register")
    suspend fun register(@Body body: Map<String, String>): ApiResponse<AuthData>

    @POST("api/auth/login")
    suspend fun login(@Body body: Map<String, String>): ApiResponse<AuthData>

    @GET("api/auth/me")
    suspend fun me(): ApiResponse<User>

    // ---------- P2P ----------
    @GET("api/p2p/config")
    suspend fun p2pConfig(): ApiResponse<P2PConfig>

    // ---------- 文件 ----------
    @GET("api/files/list")
    suspend fun listFiles(
        @Query("parent_id") parentId: Long,
        @Query("page") page: Int = 1,
        @Query("page_size") pageSize: Int = 100,
    ): ApiResponse<FileListData>

    @POST("api/files/mkdir")
    suspend fun mkdir(@Body body: Map<String, Any>): ApiResponse<FileItem>

    @POST("api/files/upload/hash")
    suspend fun uploadHash(@Body body: Map<String, Any>): ApiResponse<HashCheck>

    @Multipart
    @POST("api/files/upload")
    suspend fun upload(
        @Part file: MultipartBody.Part,
        @Part("parent_id") parentId: RequestBody,
    ): ApiResponse<FileItem>

    @Multipart
    @POST("api/files/upload/chunk")
    suspend fun uploadChunk(
        @Part file: MultipartBody.Part,
        @Part("hash") hash: RequestBody,
        @Part("chunk_index") chunkIndex: RequestBody,
        @Part("chunk_total") chunkTotal: RequestBody,
    ): ApiResponse<Map<String, Any?>>

    @POST("api/files/upload/merge")
    suspend fun uploadMerge(@Body body: Map<String, Any>): ApiResponse<FileItem>

    @Streaming
    @GET("api/files/download")
    suspend fun download(@Query("file_id") fileId: Long): ResponseBody

    @POST("api/files/rename")
    suspend fun rename(@Body body: Map<String, Any>): ApiResponse<FileItem>

    @POST("api/files/move")
    suspend fun move(@Body body: Map<String, Any>): ApiResponse<FileItem>

    @POST("api/files/delete")
    suspend fun delete(@Body body: Map<String, Any>): ApiResponse<Map<String, Any?>>

    @GET("api/files/quota")
    suspend fun quota(): ApiResponse<Quota>

    // ---------- 收藏 / 最近 / 搜索 ----------
    @POST("api/files/favorite")
    suspend fun favorite(@Body body: Map<String, Any>): ApiResponse<Map<String, Any?>>

    @POST("api/files/unfavorite")
    suspend fun unfavorite(@Body body: Map<String, Any>): ApiResponse<Map<String, Any?>>

    @GET("api/files/favorites")
    suspend fun favorites(): ApiResponse<FileListData>

    @GET("api/files/recent")
    suspend fun recent(@Query("limit") limit: Int = 50): ApiResponse<FileListData>

    @GET("api/files/search")
    suspend fun search(
        @Query("keyword") keyword: String,
        @Query("type") type: Int = -1,
    ): ApiResponse<FileListData>

    // ---------- 回收站 ----------
    @GET("api/files/trash")
    suspend fun trash(@Query("page") page: Int = 1, @Query("page_size") pageSize: Int = 100): ApiResponse<FileListData>

    @POST("api/files/restore")
    suspend fun restore(@Body body: Map<String, Any>): ApiResponse<Map<String, Any?>>

    @POST("api/files/delete/permanent")
    suspend fun deletePermanent(@Body body: Map<String, Any>): ApiResponse<Map<String, Any?>>

    @POST("api/files/trash/clear")
    suspend fun clearTrash(): ApiResponse<Map<String, Any?>>

    // ---------- 批量操作 ----------
    @POST("api/files/batch-move")
    suspend fun batchMove(@Body body: Map<String, Any>): ApiResponse<Map<String, Any?>>

    // ---------- 存储概览 ----------
    @GET("api/files/overview")
    suspend fun overview(): ApiResponse<StorageOverview>

    // ---------- 分享管理 ----------
    @GET("api/share/mine/list")
    suspend fun myShares(
        @Query("page") page: Int = 1,
        @Query("page_size") pageSize: Int = 100,
    ): ApiResponse<ShareListData>

    @HTTP(method = "DELETE", path = "api/share/mine/{id}")
    suspend fun cancelShare(@Path("id") id: Long): ApiResponse<Map<String, Any?>>

    @PUT("api/share/mine/{id}")
    suspend fun updateShare(@Path("id") id: Long, @Body body: Map<String, Any?>): ApiResponse<Map<String, Any?>>

    // ---------- 分享 ----------
    @POST("api/share/create")
    suspend fun createShare(@Body body: Map<String, Any?>): ApiResponse<ShareResult>
}
