package com.happycloud.android.ui.upload

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.Uri
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.happycloud.android.MainActivity
import com.happycloud.android.R
import com.happycloud.android.data.Network
import com.happycloud.android.util.UploadUtil
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * 后台上传前台服务：串行执行上传队列，通知栏实时显示进度。
 * 直传 / 秒传 / 分片（10MB）合并 均通过 [UploadUtil] 完成。
 */
class UploadService : Service() {

    companion object {
        const val ACTION_START = "com.happycloud.android.action.START_UPLOAD"
        const val EXTRA_URI = "extra_uri"
        const val EXTRA_NAME = "extra_name"
        const val EXTRA_SIZE = "extra_size"
        const val EXTRA_PARENT_ID = "extra_parent_id"

        private const val CHANNEL_ID = "upload_channel"
        private const val NOTIFICATION_ID = 1001

        fun start(context: Context, uri: Uri, name: String, size: Long, parentId: Long) {
            val intent = Intent(context, UploadService::class.java).apply {
                action = ACTION_START
                putExtra(EXTRA_URI, uri)
                putExtra(EXTRA_NAME, name)
                putExtra(EXTRA_SIZE, size)
                putExtra(EXTRA_PARENT_ID, parentId)
            }
            ContextCompat.startForegroundService(context, intent)
        }
    }

    private data class PendingUpload(val uri: Uri, val name: String, val size: Long, val parentId: Long)

    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val queue = ArrayDeque<PendingUpload>()
    private var running = false
    private var currentJob: Job? = null
    private lateinit var notificationManager: NotificationManager

    override fun onCreate() {
        super.onCreate()
        notificationManager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        createChannel()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_START) {
            val uri: Uri? = intent.getParcelableExtra(EXTRA_URI)
            val name = intent.getStringExtra(EXTRA_NAME) ?: "未命名文件"
            val size = intent.getLongExtra(EXTRA_SIZE, 0L)
            val parentId = intent.getLongExtra(EXTRA_PARENT_ID, 0L)
            if (uri != null) {
                queue.addLast(PendingUpload(uri, name, size, parentId))
                processQueue()
            }
        }
        return START_NOT_STICKY
    }

    private fun processQueue() {
        if (running) return
        running = true
        val next = queue.removeFirstOrNull()
        if (next == null) {
            running = false
            stopSelfQuietly()
            return
        }
        currentJob = serviceScope.launch {
            runUpload(next)
            processQueue()
        }
    }

    private suspend fun runUpload(upload: PendingUpload) {
        val taskId = UploadManager.add(upload.uri, upload.name, upload.size, upload.parentId)
        val baseNotification = buildNotification(
            title = "正在上传「${upload.name}」",
            progress = 0,
        )
        startAsForeground(baseNotification)

        try {
            UploadManager.update(taskId) { it.copy(status = UploadManager.Status.UPLOADING, progress = 0) }
            val result = UploadUtil.uploadFile(
                context = applicationContext,
                api = Network.api,
                uri = upload.uri,
                name = upload.name,
                size = upload.size,
                parentId = upload.parentId,
            ) { progress ->
                val percent = (progress * 100).toInt().coerceIn(0, 100)
                serviceScope.launch {
                    UploadManager.update(taskId) { it.copy(progress = percent) }
                    notificationManager.notify(
                        NOTIFICATION_ID,
                        buildNotification(
                            title = "正在上传「${upload.name}」",
                            progress = percent,
                        ),
                    )
                }
            }
            UploadManager.update(taskId) {
                it.copy(status = UploadManager.Status.SUCCESS, progress = 100)
            }
            notifyFinished(
                title = if (result == null) "秒传成功「${upload.name}」" else "上传完成「${upload.name}」",
            )
        } catch (e: Exception) {
            val msg = e.message ?: "未知错误"
            UploadManager.update(taskId) {
                it.copy(status = UploadManager.Status.FAILED, message = msg)
            }
            notifyFinished(title = "上传失败「${upload.name}」", text = msg, failed = true)
        } finally {
            currentJob = null
        }
    }

    private fun startAsForeground(notification: Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ServiceCompat.startForeground(
                this, NOTIFICATION_ID, notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC,
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun notifyFinished(title: String, text: String? = null, failed: Boolean = false) {
        val notification = NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_launcher_foreground)
            .setContentTitle(title)
            .setContentText(text ?: "点击查看上传任务")
            .setAutoCancel(true)
            .setContentIntent(contentPendingIntent())
            .build()
        notificationManager.notify(NOTIFICATION_ID, notification)
        // 无排队任务时结束前台状态
        if (queue.isEmpty()) {
            ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_DETACH)
        }
    }

    private fun buildNotification(title: String, progress: Int): Notification {
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_launcher_foreground)
            .setContentTitle(title)
            .setContentText("进度 ${progress}%")
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setProgress(100, progress, false)
            .setContentIntent(contentPendingIntent())
            .build()
    }

    private fun contentPendingIntent(): PendingIntent {
        val intent = Intent(this, MainActivity::class.java)
        return PendingIntent.getActivity(
            this, 0, intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun createChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID, "上传任务",
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = "显示后台上传任务进度"
        }
        notificationManager.createNotificationChannel(channel)
    }

    private fun stopSelfQuietly() {
        try {
            stopSelf()
        } catch (_: Exception) {
        }
    }

    override fun onDestroy() {
        serviceScope.cancel()
        super.onDestroy()
    }
}
