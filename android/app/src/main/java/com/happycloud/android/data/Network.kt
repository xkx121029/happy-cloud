package com.happycloud.android.data

import com.google.gson.GsonBuilder
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.util.concurrent.TimeUnit

/** 网络层：Retrofit + OkHttp + JWT 拦截器 */
object Network {

    /** 后端地址（模拟器访问宿主机用 10.0.2.2，真机改为局域网 IP） */
    const val BASE_URL = "http://10.0.2.2:8080/"

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

    private val retrofit: Retrofit by lazy {
        Retrofit.Builder()
            .baseUrl(BASE_URL)
            .client(okHttpClient)
            .addConverterFactory(GsonConverterFactory.create(GsonBuilder().create()))
            .build()
    }

    val api: ApiService by lazy { retrofit.create(ApiService::class.java) }
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
