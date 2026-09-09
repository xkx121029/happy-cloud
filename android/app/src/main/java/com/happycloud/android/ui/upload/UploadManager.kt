package com.happycloud.android.ui.upload

import android.net.Uri
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** 后台上传任务管理（内存态，App 进程存活期间有效） */
object UploadManager {

    enum class Status { QUEUED, UPLOADING, SUCCESS, FAILED }

    data class Task(
        val id: Long,
        val uri: Uri,
        val name: String,
        val size: Long,
        val parentId: Long,
        val status: Status = Status.QUEUED,
        val progress: Int = 0,
        val message: String? = null,
    )

    private val _tasks = MutableStateFlow<List<Task>>(emptyList())
    val tasks: StateFlow<List<Task>> = _tasks.asStateFlow()

    private val lock = Mutex()
    private var nextId = 1L

    suspend fun add(uri: Uri, name: String, size: Long, parentId: Long): Long = lock.withLock {
        val id = nextId++
        _tasks.update { it + Task(id = id, uri = uri, name = name, size = size, parentId = parentId) }
        id
    }

    suspend fun update(id: Long, transform: (Task) -> Task) = lock.withLock {
        _tasks.update { list ->
            list.map { if (it.id == id) transform(it) else it }
        }
    }

    suspend fun clearFinished() = lock.withLock {
        _tasks.update { list -> list.filter { it.status == Status.QUEUED || it.status == Status.UPLOADING } }
    }
}
