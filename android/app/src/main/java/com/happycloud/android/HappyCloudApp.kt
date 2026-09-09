package com.happycloud.android

import android.app.Application
import android.content.Context
import androidx.compose.runtime.staticCompositionLocalOf
import com.happycloud.android.data.Network
import com.happycloud.android.data.TokenStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/** 手动依赖注入容器 */
class AppContainer(context: Context) {
    val tokenStore = TokenStore(context.applicationContext)
    val api = Network.api

    init {
        Network.tokenProvider = { tokenStore.cachedToken }
    }

    /** 启动时从 DataStore 恢复登录态到内存缓存 */
    fun restoreSession(scope: CoroutineScope) {
        scope.launch {
            tokenStore.cachedToken = tokenStore.getToken()
        }
    }
}

/** 全局 CompositionLocal，供各页面获取容器 */
val LocalAppContainer = staticCompositionLocalOf<AppContainer> {
    error("AppContainer 未提供")
}

class HappyCloudApp : Application() {

    lateinit var container: AppContainer
        private set

    private val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
        container.restoreSession(appScope)
    }
}
