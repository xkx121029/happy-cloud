package com.happycloud.android.data.p2p

import android.content.Context
import android.os.Handler
import android.os.HandlerThread
import com.happycloud.android.data.p2p.ConnMode.HTTP
import com.happycloud.android.data.p2p.ConnMode.P2P
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import org.json.JSONObject
import org.webrtc.DataChannel
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.PeerConnection
import org.webrtc.PeerConnectionFactory
import org.webrtc.RtpReceiver
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription
import java.util.concurrent.TimeoutException
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** 当前连接模式 */
enum class ConnMode { HTTP, P2P }

private class Pending(
    val resolve: (String?) -> Unit,
    val reject: (Throwable) -> Unit,
)

/** 下载流回调 */
class StreamCb(
    val onMeta: (JSONObject) -> Unit,
    val onData: (ByteArray, Int) -> Unit,
    val onDone: () -> Unit,
    val onError: (String) -> Unit,
)

/**
 * 隧道传输：Android 端 WebRTC DataChannel 到后端（offerer 角色）。
 * 负责信令协商、R 握手鉴权，以及隧道内 JSON 代理 / 下载流 / 上传分片。
 * 与 Web src/p2p/transport.ts 行为对齐。
 */
class TunnelTransport(
    private val appContext: Context,
    private val okHttpClient: OkHttpClient,
) {

    @Volatile var mode: ConnMode = HTTP
    @Volatile var connected: Boolean = false

    private var factory: PeerConnectionFactory? = null
    private var pc: PeerConnection? = null
    private var dc: DataChannel? = null
    private var signaling: SignalingClient? = null
    private val decoder = FrameDecoder()
    private var seq = 1
    private val pending = HashMap<Int, Pending>()
    private val streams = HashMap<Int, StreamCb>()
    private var token = ""
    private var clientId = ""

    private var thread: HandlerThread? = null
    private var handler: Handler? = null

    private var handshakeDone = false
    private var handshakeFinish: (() -> Unit)? = null
    private var handshakeReject: ((Throwable) -> Unit)? = null
    private var handshakeTimer: Runnable? = null

    private val closing = AtomicBoolean(false)

    private fun nextId() = seq++ and 0xffffff

    /** 建立 P2P 隧道：协商 + R 握手成功返回 true；失败/false。 */
    suspend fun connect(wsUrl: String, token: String, iceUrls: List<String>): Boolean {
        this.token = token
        return withContext(Dispatchers.IO) {
            suspendCancellableCoroutine { cont ->
                post {
                    val ok = startNegotiation(wsUrl, iceUrls)
                    if (!ok) cont.resume(false) else cont.resume(true)
                }
            }
        }
    }

    private fun ensureExecutor() {
        if (thread != null) return
        thread = HandlerThread("happycloud-p2p").also { it.start() }
        handler = Handler(thread!!.looper)
        ensureFactory()
    }

    private fun ensureFactory() {
        if (factory != null) return
        synchronized(LOCK) {
            if (factory != null) return
            val initOpts = PeerConnectionFactory.InitializationOptions.builder(appContext)
                .createInitializationOptions()
            PeerConnectionFactory.initialize(initOpts)
            factory = PeerConnectionFactory.builder().createPeerConnectionFactory()
        }
    }

    private fun post(r: () -> Unit) {
        ensureExecutor()
        handler?.post(r)
    }

    private fun startNegotiation(wsUrl: String, iceUrls: List<String>): Boolean {
        val f = factory ?: return false
        val slashIce = iceUrls.map { PeerConnection.IceServer.builder(it).createIceServer() }
        val rtcConfig = PeerConnection.RTCConfiguration(slashIce)
        var failed = false

        val pc = f.createPeerConnection(rtcConfig, object : PeerConnection.Observer {
            override fun onSignalingChange(state: PeerConnection.SignalingState) {}
            override fun onIceConnectionChange(state: PeerConnection.IceConnectionState) {
                if (state == PeerConnection.IceConnectionState.CONNECTED) mode = P2P
            }
            override fun onIceConnectionReceivingChange(receiving: Boolean) {}
            override fun onIceGatheringChange(state: PeerConnection.IceGatheringState) {}
            override fun onIceCandidate(candidate: IceCandidate) {
                if (clientId.isEmpty()) return
                this@TunnelTransport.signaling?.send(
                    JSONObject()
                        .put("t", "ice")
                        .put("cid", clientId)
                        .put(
                            "cand", JSONObject()
                                .put("candidate", candidate.sdp)
                                .put("sdpMid", candidate.sdpMid)
                                .put("sdpMLineIndex", candidate.sdpMLineIndex)
                        )
                )
            }
            override fun onIceCandidatesRemoved(candidates: Array<out IceCandidate>) {}
            override fun onAddStream(stream: MediaStream) {}
            override fun onRemoveStream(stream: MediaStream) {}
            override fun onDataChannel(channel: DataChannel) {}
            override fun onRenegotiationNeeded() {}
            override fun onAddTrack(receiver: RtpReceiver, tracks: Array<out MediaStream>) {}
        }) ?: return false
        this.pc = pc

        val dc = pc.createDataChannel("tunnel", DataChannel.Init())
        this.dc = dc
        setupChannel(dc)

        val signaling = SignalingClient(okHttpClient, wsUrl)
        this.signaling = signaling
        signaling.connect(object : SignalingClient.Listener {
            override fun onMessage(msg: JSONObject) {
                when (msg.optString("t")) {
                    "welcome" -> {
                        clientId = msg.optString("id")
                        createOfferAndSend()
                    }
                    "answer" -> {
                        val sdp = msg.optJSONObject("sdp") ?: return
                        pc.setRemoteDescription(
                            noopObserver,
                            SessionDescription(SessionDescription.Type.ANSWER, sdp.optString("sdp")),
                        )
                    }
                    "ice" -> {
                        val cand = msg.optJSONObject("cand") ?: return
                        try {
                            pc.addIceCandidate(
                                IceCandidate(
                                    cand.optString("sdpMid"),
                                    cand.optInt("sdpMLineIndex"),
                                    cand.optString("candidate"),
                                )
                            )
                        } catch (_: Exception) {
                        }
                    }
                    "replace", "host-offline" -> {
                        if (!connected) failed = true
                    }
                }
            }

            override fun onClose() {
                if (!failed && !connected && !closing.get()) {
                    failed = true
                    cleanup()
                }
            }
        })

        // 发起协商失败兜底
        handler?.postDelayed({
            if (!connected && !failed) {
                failed = true
                cleanup()
            }
        }, 20000)

        return true
    }

    /** 信令就绪后创建 offer，setLocalDescription 后经信令发给 host */
    private fun createOfferAndSend() {
        val pc = pc ?: return
        val constraints = MediaConstraints()
        pc.createOffer(object : SdpObserver {
            override fun onCreateSuccess(desc: SessionDescription) {
                pc.setLocalDescription(object : SdpObserver {
                    override fun onSetSuccess() {
                        if (clientId.isEmpty()) return
                        signaling?.send(
                            JSONObject()
                                .put("t", "offer")
                                .put("cid", clientId)
                                .put("sdp", JSONObject().put("type", "offer").put("sdp", desc.description))
                        )
                    }
                    override fun onCreateSuccess(desc: SessionDescription) {}
                    override fun onCreateFailure(error: String) {}
                    override fun onSetFailure(error: String) {}
                }, desc)
            }
            override fun onCreateFailure(error: String) {}
            override fun onSetFailure(error: String) {}
            override fun onSetSuccess() {}
        }, constraints)
    }

    private fun setupChannel(dc: DataChannel) {
        dc.registerObserver(object : DataChannel.Observer {
            override fun onBufferedAmountChange(previousAmount: Long) {}
            override fun onStateChange() {
                if (dc.state() == DataChannel.State.OPEN) {
                    // R 握手：token
                    sendFrame(JSONObject().put("t", "R").put("id", nextId()).put("b", token))
                    armHandshake()
                }
            }
            override fun onMessage(buffer: DataChannel.Buffer) {
                handleChannelMessage(buffer)
            }
        })
    }

    private fun handleChannelMessage(buffer: DataChannel.Buffer) {
        val bin = ByteArray(buffer.data.remaining()).also { buffer.data.get(it) }
        val parsed = decoder.feed(bin) ?: return
        handleFrame(parsed.env, parsed.bin)
    }

    private fun handleFrame(env: JSONObject, bin: ByteArray) {
        when (env.optString("t")) {
            "R" -> {
                if (env.optBoolean("ok")) {
                    handshakeDone = true
                    connected = true
                    mode = P2P
                    handshakeTimer?.let { handler?.removeCallbacks(it) }
                    handshakeFinish?.invoke()
                } else {
                    handshakeDone = true
                    handshakeTimer?.let { handler?.removeCallbacks(it) }
                    handshakeReject?.invoke(RuntimeException(env.optString("msg").ifBlank { "P2P 鉴权失败" }))
                }
            }
            "D" -> {
                val id = env.optInt("id")
                val cb = if (env.has("id")) streams[id] else null
                if (cb == null) return
                if (bin.isNotEmpty()) {
                    cb.onData(bin, env.optInt("s").let { if (it > 0) it else bin.size })
                } else if (env.optBoolean("l")) {
                    cb.onDone()
                    streams.remove(id)
                } else if (env.has("b")) {
                    // 元信息首帧
                    try {
                        val body = env.get("b")
                        val meta = if (body is String) JSONObject(body) else env.optJSONObject("b")
                        if (meta != null) {
                            cb.onMeta(meta)
                            sendFrame(JSONObject().put("t", "A").put("id", id).put("c", 0))
                        }
                    } catch (_: Exception) {
                    }
                }
                if (bin.isNotEmpty()) sendFrame(JSONObject().put("t", "A").put("id", id).put("c", 0))
            }
            "E" -> {
                val id = env.optInt("id")
                pending.remove(id)?.let { it.reject(RuntimeException(env.optString("msg").ifBlank { "P2P 错误" })) }
                if (env.has("id")) streams.remove(id)?.let { it.onError(env.optString("msg").ifBlank { "传输错误" }) }
            }
            "J", "A" -> {
                val id = env.optInt("id")
                val p = pending.remove(id) ?: return
                val body = if (env.has("b") && !env.isNull("b")) env.get("b").toString() else null
                p.resolve(body)
            }
        }
    }

    /**
     * 隧道内 JSON HTTP 代理：返回后端 `{code,message,data}` 的 JSON 字符串。
     * 调用方自行解析；后端的业务失败恒为 HTTP 200 + code!=0，需结合 body 判断。
     */
    suspend fun requestAwait(
        method: String,
        path: String,
        query: Map<String, String>? = null,
        body: String? = null,
    ): String = withContext(Dispatchers.IO) {
        suspendCancellableCoroutine { cont ->
            post {
                if (!connected) {
                    cont.resumeWithException(RuntimeException("P2P 未连接"))
                    return@post
                }
                val id = nextId()
                val env = JSONObject().put("t", "j").put("id", id)
                    .put("m", method).put("p", path)
                if (query != null && query.isNotEmpty()) env.put("q", JSONObject(query))
                if (body != null) env.put("b", body)
                pending[id] = Pending({ r -> cont.resume(r ?: "{}") }, { e -> cont.resumeWithException(e) })
                sendFrame(env)
            }
        }
    }

    /** 开启文件下载流（d 帧）；之后由 StreamCb 回调数据 */
    fun downloadStream(fileId: Long, cb: StreamCb) {
        val id = nextId()
        streams[id] = cb
        sendFrame(JSONObject().put("t", "d").put("id", id).put("dl", fileId))
    }

    /** 上传一个分片（U 帧），等服务端 A 确认 */
    suspend fun uploadChunk(hash: String, idx: Int, bin: ByteArray) {
        withContext(Dispatchers.IO) {
            suspendCancellableCoroutine { cont ->
                post {
                    if (!connected) {
                        cont.resumeWithException(RuntimeException("P2P 未连接"))
                        return@post
                    }
                    val id = nextId()
                    pending[id] = Pending({ cont.resume(Unit) }, { e -> cont.resumeWithException(e) })
                    sendFrame(
                        JSONObject()
                            .put("t", "U").put("id", id)
                            .put("hash", hash).put("idx", idx).put("s", bin.size),
                        bin,
                    )
                }
            }
        }
    }

    private fun armHandshake() {
        handshakeDone = false
        handshakeTimer?.let { handler?.removeCallbacks(it) }
        val timer = Runnable {
            if (!handshakeDone && !connected) {
                handshakeDone = true
                handshakeReject?.invoke(TimeoutException("P2P 握手超时"))
            }
        }
        handshakeTimer = timer
        handler?.postDelayed(timer, 15000)
    }

    private fun sendFrame(env: JSONObject, bin: ByteArray? = null) {
        post {
            val d = dc
            if (d?.state() != DataChannel.State.OPEN) return@post
            for (pkt in encodeFrame(env, bin)) {
                d.send(DataChannel.Buffer(java.nio.ByteBuffer.wrap(pkt), true))
            }
        }
    }

    fun close() {
        closing.set(true)
        cleanup()
        connected = false
        mode = HTTP
    }

    private fun cleanup() {
        try {
            signaling?.close()
        } catch (_: Exception) {
        }
        try {
            dc?.close()
        } catch (_: Exception) {
        }
        try {
            pc?.close()
        } catch (_: Exception) {
        }
    }

    private val noopObserver = object : SdpObserver {
        override fun onCreateSuccess(desc: SessionDescription) {}
        override fun onCreateFailure(error: String) {}
        override fun onSetFailure(error: String) {}
        override fun onSetSuccess() {}
    }

    companion object {
        private val LOCK = Any()
    }
}