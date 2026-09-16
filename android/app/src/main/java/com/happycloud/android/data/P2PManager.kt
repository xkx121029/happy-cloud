package com.happycloud.android.data

import android.content.Context
import com.happycloud.android.data.p2p.ConnMode
import com.happycloud.android.data.p2p.ConnMode.HTTP
import com.happycloud.android.data.p2p.ConnMode.P2P
import com.happycloud.android.data.p2p.TunnelTransport
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import java.util.concurrent.atomic.AtomicBoolean

/**
 * P2P 打洞连接管理：负责拉起 WebRTC 隧道到后端（局域网直连/穿透直连），
 * 并把当前连接模式（HTTP / P2P）作为状态暴露给 UI。
 * 其余请求仍走 HTTP 直连；隧道建立成功后可作为加速/穿透路径。
 */
class P2PManager(private val context: Context) {

    val transport = TunnelTransport(context, wsClient)

    private val _mode = MutableStateFlow<ConnMode>(HTTP)
    val mode: StateFlow<ConnMode> = _mode.asStateFlow()

    private val _status = MutableStateFlow("未连接")
    val status: StateFlow<String> = _status.asStateFlow()

    private val started = AtomicBoolean(false)

    /** 由 AppContainer 注入当前 JWT */
    var tokenProvider: (() -> String?)? = null

    private val api get() = Network.api

    /** 拉取 P2P 配置并建立隧道（仅触发一次，失败不阻塞主流程） */
    fun ensureConnected(scope: CoroutineScope) {
        if (started.getAndSet(true)) return
        scope.launch {
            val token = tokenProvider?.invoke()
            if (token.isNullOrBlank()) {
                _status.value = "未连接（未登录）"
                return@launch
            }
            try {
                _status.value = "正在打洞…"
                val cfg = safeApi { api.p2pConfig() }
                if (!cfg.p2p || cfg.ws.isBlank()) {
                    _status.value = "P2P 未启用"
                    return@launch
                }
                val iceUrls = cfg.iceServers.mapNotNull { s -> s.urls.firstOrNull() }
                val ok = transport.connect(cfg.ws, token, iceUrls)
                _mode.value = if (ok) P2P else HTTP
                _status.value = if (ok) "直连(打洞)" else "P2P 失败，切回直连"
            } catch (e: Exception) {
                _mode.value = HTTP
                _status.value = "打洞失败(${e.message ?: "未知错误"})"
            }
        }
    }

    fun close() {
        transport.close()
        _mode.value = HTTP
        _status.value = "已断开"
        started.set(false)
    }

    companion object {
        private val wsClient: OkHttpClient = OkHttpClient.Builder().build()
    }
}