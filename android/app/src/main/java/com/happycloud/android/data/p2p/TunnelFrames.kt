package com.happycloud.android.data.p2p

import org.json.JSONObject
import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * 隧道帧编解码：与后端 internal/p2p/frames.go、Web src/p2p/frames.ts 对齐。
 * 逻辑消息流 = [4B BE envLen][envJSON][bin]；超过 FRAG_LIMIT 拆成多 packet，
 * 每个 packet = [4B BE fragLen][fragJSON][4B BE chunkLen][chunk]，带 f/fn 供重组。
 */
const val FRAG_LIMIT = 16000

/** 封装 WebRTC DataChannel 上传输的一个 packet：[4B fragLen][fragJSON][4B chunkLen][chunk] */
private fun buildPacket(fragJson: ByteArray, chunk: ByteArray): ByteArray {
    val buf = ByteBuffer.allocate(8 + fragJson.size + chunk.size).order(ByteOrder.BIG_ENDIAN)
    buf.putInt(fragJson.size)
    buf.put(fragJson)
    buf.putInt(chunk.size)
    buf.put(chunk)
    return buf.array()
}

/**
 * 把一条逻辑消息编码为若干个 packet。
 * @param env JSON 信封（含 t/id 等字段，payload 直接放 b 里）
 * @param bin 可选二进制负载
 */
fun encodeFrame(env: JSONObject, bin: ByteArray? = null): List<ByteArray> {
    val envB = env.toString().toByteArray(Charsets.UTF_8)
    val stream = ByteBuffer.allocate(4 + envB.size + (bin?.size ?: 0)).order(ByteOrder.BIG_ENDIAN)
        .putInt(envB.size).put(envB).also { if (bin != null) it.put(bin) }.array()

    val t = env.optString("t")
    val hasId = env.has("id")
    val id = env.optInt("id")

    if (stream.size <= FRAG_LIMIT) {
        val frag = JSONObject().put("t", t)
        if (hasId) frag.put("id", id)
        return listOf(buildPacket(frag.toString().toByteArray(Charsets.UTF_8), stream))
    }
    val n = (stream.size + FRAG_LIMIT - 1) / FRAG_LIMIT
    val out = ArrayList<ByteArray>(n)
    for (i in 0 until n) {
        val lo = i * FRAG_LIMIT
        val hi = minOf(lo + FRAG_LIMIT, stream.size)
        val frag = JSONObject().put("t", t)
        if (hasId) frag.put("id", id)
        frag.put("f", i).put("fn", n)
        out.add(buildPacket(frag.toString().toByteArray(Charsets.UTF_8), stream.copyOfRange(lo, hi)))
    }
    return out
}

/** 一条重组完成、可处理的逻辑消息 */
data class ParsedFrame(val env: JSONObject, val bin: ByteArray)

/** 聚合器：把 DataChannel 收到的 packet 重组成完整逻辑消息 */
class FrameDecoder {

    private class FragState(var bytes: ByteArray = ByteArray(0), var count: Int = 0, val need: Int)

    private val partial = HashMap<String, FragState>()

    /** feed 一个 packet；若拼出完整逻辑消息则返回，否则返回 null */
    fun feed(pkt: ByteArray): ParsedFrame? {
        if (pkt.size < 8) return null
        val buf = ByteBuffer.wrap(pkt).order(ByteOrder.BIG_ENDIAN)
        val fragLen = buf.int
        if (pkt.size < 4 + fragLen + 4) return null
        var frag: JSONObject
        try {
            frag = JSONObject(String(pkt, 4, fragLen, Charsets.UTF_8))
        } catch (_: Exception) {
            return null
        }
        val chunkLen = ByteBuffer.wrap(pkt, 4 + fragLen, 4).order(ByteOrder.BIG_ENDIAN).int
        if (pkt.size < 8 + fragLen + chunkLen) return null
        val chunk = pkt.copyOfRange(8 + fragLen, 8 + fragLen + chunkLen)

        if (!frag.has("fn")) return parseStream(chunk)
        val key = "${frag.optString("t")}|${frag.optInt("id")}"
        var st = partial[key]
        if (st == null) {
            st = FragState(need = frag.optInt("fn"))
            partial[key] = st
        }
        st.bytes = st.bytes + chunk
        st.count++
        if (st.count >= st.need) {
            partial.remove(key)
            return parseStream(st.bytes)
        }
        return null
    }

    /** 解析完整逻辑流：[4B envLen][envJSON][bin] */
    private fun parseStream(stream: ByteArray): ParsedFrame? {
        if (stream.size < 4) return null
        val msgLen = ByteBuffer.wrap(stream, 0, 4).order(ByteOrder.BIG_ENDIAN).int
        if (stream.size < 4 + msgLen) return null
        var env: JSONObject
        try {
            env = JSONObject(String(stream, 4, msgLen, Charsets.UTF_8))
        } catch (_: Exception) {
            return null
        }
        val bin = stream.copyOfRange(4 + msgLen, stream.size)
        return ParsedFrame(env, bin)
    }
}