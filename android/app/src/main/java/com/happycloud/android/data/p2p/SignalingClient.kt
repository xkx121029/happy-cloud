package com.happycloud.android.data.p2p

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.util.concurrent.atomic.AtomicBoolean

/**
 * 独立信令服务器的 WebSocket 客户端（client 角色）。
 * 仅透传 SDP/ICE/welcome/ping 等，按 room 与后端 host 配对；
 * 鉴权由后端在 DataChannel 握手（R 帧）时完成，信令侧不涉及任何业务密钥。
 */
class SignalingClient(private val okHttpClient: OkHttpClient, private val url: String) {

    interface Listener {
        fun onMessage(msg: JSONObject)
        fun onClose()
    }

    private var ws: WebSocket? = null
    private val closedByUser = AtomicBoolean(false)

    /** 建立连接；消息/断开都通过 listener 回调（在 OkHttp 线程上） */
    fun connect(listener: Listener) {
        closedByUser.set(false)
        val request = Request.Builder().url(url).build()
        ws = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) {
                try {
                    listener.onMessage(JSONObject(text))
                } catch (_: Exception) {
                    // 忽略非 JSON 帧
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (!closedByUser.get()) listener.onClose()
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (!closedByUser.get()) listener.onClose()
            }
        })
    }

    fun send(msg: JSONObject) {
        ws?.send(msg.toString())
    }

    fun close() {
        closedByUser.set(true)
        try {
            ws?.close(1000, "bye")
        } catch (_: Exception) {
        }
    }
}