package com.happycloud.android.data

import com.google.gson.GsonBuilder
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.util.concurrent.TimeUnit

/** 网络层：Retrofit + OkHttp + JWT 拦截器，支持运行时切换服务端地址 */
object Network {

    /** 默认后端地址（模拟器访问宿主机用 10.0.2.2，真机可改局域网 IP） */
    const val DEFAULT_BASE_URL = "http://10.0.2.2:8080/"

    /** 当前服务端地址（以 / 结尾），登录页可自定义 */
    @Volatile
    var baseUrl: String = DEFAULT_BASE_URL
        private set

    /** 由 AppContainer 注入：返回当前 JWT，用于所有请求头 */
    @Volatile
    var tokenProvider: (() -> String?)? = null

    private val okHttpClient: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(20, TimeUnit.SECONDS)
            .readTimeout(120, TimeUnit.SECONDS)
            .writeTimeout(120, TimeUnit.SECONDS)
            .addInterceptor { chain ->
                val token = tokenProvider?.invoke()
                val request = if (!token.isNullOrBlank()) {
                    chain.request().newBuilder()
                        .header("Authorization", "Bearer $token")
                        .build()
                } else {
                    chain.request()
                }
                chain.proceed(request)
            }
            .addInterceptor(
                HttpLoggingInterceptor().apply { level = HttpLoggingInterceptor.Level.BASIC }
            )
            .build()
    }

    @Volatile
    private var apiInstance: ApiService? = null

    /** 切换服务端地址并重建 Retrofit；登录/设置服务器地址时调用 */
    @Synchronized
    fun setBaseUrl(url: String) {
        val trimmed = url.trim().trimEnd('/')
        val normalized = if (trimmed.isEmpty()) DEFAULT_BASE_URL else "$trimmed/"
        baseUrl = normalized
        apiInstance = null
    }

    /** 懒创建 Retrofit；地址变更后自动重建 */
    val api: ApiService
        get() = synchronized(this) {
            apiInstance?.let { return it }
            val retrofit = Retrofit.Builder()
                .baseUrl(baseUrl)
                .client(okHttpClient)
                .addConverterFactory(GsonConverterFactory.create(GsonBuilder().create()))
                .build()
            retrofit.create(ApiService::class.java).also { apiInstance = it }
        }
}

/** 业务异常：code 非 0 或网络错误（已转中文提示） */
class ApiException(val code: Int, override val message: String) : Exception(message)

/** 统一调用：校验 {code,message,data} 包装 */
suspend fun <T> safeApi(call: suspend () -> ApiResponse<T>): T {
    return try {
        val resp = call()
        if (resp.code != 0) throw ApiException(resp.code, resp.message.ifBlank { "操作失败" })
        resp.data ?: throw ApiException(-1, "返回数据为空")
    } catch (e: ApiException) {
        throw e
    } catch (e: java.io.IOException) {
        throw ApiException(-2, "网络连接失败，请检查网络与服务器地址")
    } catch (e: retrofit2.HttpException) {
        throw ApiException(e.code(), "服务器错误（${e.code()}）")
    }
}
