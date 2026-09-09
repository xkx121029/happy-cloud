package com.happycloud.android.ui.files

import android.content.Context
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.happycloud.android.AppContainer
import com.happycloud.android.data.ApiException
import com.happycloud.android.data.FileItem
import com.happycloud.android.data.safeApi
import com.happycloud.android.util.DownloadUtil
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 面包屑节点 */
data class Breadcrumb(val parentId: Long, val name: String)

class FilesViewModel(
    private val container: AppContainer,
    private val appContext: Context,
) : ViewModel() {

    data class UiState(
        val items: List<FileItem> = emptyList(),
        val breadcrumb: List<Breadcrumb> = listOf(Breadcrumb(0, "全部文件")),
        val quotaMax: Long = 0,
        val quotaUsed: Long = 0,
        val loading: Boolean = false,
        val refreshing: Boolean = false,
        val gridMode: Boolean = true,
        val error: String? = null,
        val snackbar: String? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state.asStateFlow()

    val currentParentId: Long get() = _state.value.breadcrumb.last().parentId

    init {
        load(currentParentId, refreshing = false)
    }

    fun refresh() = load(currentParentId, refreshing = true)

    private fun load(parentId: Long, refreshing: Boolean) {
        viewModelScope.launch {
            _state.update {
                if (refreshing) it.copy(refreshing = true, error = null)
                else it.copy(loading = true, error = null)
            }
            try {
                val list = safeApi { container.api.listFiles(parentId) }
                val quota = safeApi { container.api.quota() }
                _state.update {
                    it.copy(
                        items = list.items,
                        quotaMax = quota.quotaMax,
                        quotaUsed = quota.quotaUsed,
                        loading = false,
                        refreshing = false,
                        error = null,
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    it.copy(loading = false, refreshing = false, error = e.message)
                }
            }
        }
    }

    fun enterFolder(item: FileItem) {
        if (!item.isFolder) return
        _state.update { it.copy(breadcrumb = it.breadcrumb + Breadcrumb(item.id, item.name)) }
        load(item.id, refreshing = false)
    }

    /** 面包屑点击跳转 */
    fun navigateTo(index: Int) {
        val bc = _state.value.breadcrumb.take(index + 1)
        val last = bc.last()
        _state.update { it.copy(breadcrumb = bc) }
        load(last.parentId, refreshing = false)
    }

    fun toggleGrid(grid: Boolean) = _state.update { it.copy(gridMode = grid) }

    fun consumeSnackbar() = _state.update { it.copy(snackbar = null) }

    // ---------- 文件操作 ----------

    fun mkdir(name: String, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi {
                    container.api.mkdir(mapOf("parent_id" to currentParentId, "name" to name.trim()))
                }
                refresh()
                _state.update { it.copy(snackbar = "已创建文件夹「${name.trim()}」") }
                onDone()
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
                onDone()
            }
        }
    }

    fun rename(file: FileItem, newName: String, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi {
                    container.api.rename(mapOf("file_id" to file.id, "new_name" to newName.trim()))
                }
                refresh()
                _state.update { it.copy(snackbar = "已重命名为「${newName.trim()}」") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    fun move(file: FileItem, targetParentId: Long, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi {
                    container.api.move(
                        mapOf("file_id" to file.id, "target_parent_id" to targetParentId)
                    )
                }
                refresh()
                _state.update { it.copy(snackbar = "已移动「${file.name}」") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    fun delete(file: FileItem, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi {
                    container.api.delete(mapOf("file_ids" to listOf(file.id)))
                }
                refresh()
                _state.update { it.copy(snackbar = "已删除「${file.name}」") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    fun download(file: FileItem) {
        viewModelScope.launch {
            try {
                val msg = DownloadUtil.download(appContext, container.api, file.id, file.name)
                _state.update { it.copy(snackbar = msg) }
            } catch (e: Exception) {
                val msg = (e as? ApiException)?.message ?: "下载失败：${e.message}"
                _state.update { it.copy(snackbar = msg) }
            }
        }
    }
}

fun filesViewModelFactory(container: AppContainer): ViewModelProvider.Factory = viewModelFactory {
    initializer {
        val appContext = checkNotNull(this[ViewModelProvider.AndroidViewModelFactory.APPLICATION_KEY])
        FilesViewModel(container, appContext)
    }
}
