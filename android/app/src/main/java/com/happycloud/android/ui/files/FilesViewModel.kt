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

/** 列表数据来源 */
enum class FileSource { ALL, RECENT, FAVORITE }

class FilesViewModel(
    private val container: AppContainer,
    private val appContext: Context,
) : ViewModel() {

    data class UiState(
        val source: FileSource = FileSource.ALL,
        val items: List<FileItem> = emptyList(),
        val breadcrumb: List<Breadcrumb> = listOf(Breadcrumb(0, "全部文件")),
        val quotaMax: Long = 0,
        val quotaUsed: Long = 0,
        val overview: com.happycloud.android.data.StorageOverview? = null,
        val favoriteIds: Set<Long> = emptySet(),
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
                val source = _state.value.source
                val list = when (source) {
                    FileSource.ALL -> safeApi { container.api.listFiles(parentId) }
                    FileSource.RECENT -> safeApi { container.api.recent(50) }
                    FileSource.FAVORITE -> safeApi { container.api.favorites() }
                }
                val quota = safeApi { container.api.quota() }
                // 收藏与概览只在进入页面/下拉刷新时加载，避免频繁请求
                val favIds = loadFavoriteIds()
                val overview = loadOverview()
                _state.update {
                    it.copy(
                        items = list.items,
                        quotaMax = quota.quotaMax,
                        quotaUsed = quota.quotaUsed,
                        favoriteIds = favIds,
                        overview = overview,
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

    private suspend fun loadFavoriteIds(): Set<Long> {
        return try {
            safeApi { container.api.favorites() }.items.map { it.id }.toSet()
        } catch (_: Exception) {
            emptySet()
        }
    }

    private suspend fun loadOverview(): com.happycloud.android.data.StorageOverview? {
        return try {
            safeApi { container.api.overview() }
        } catch (_: Exception) {
            null
        }
    }

    /** 切换数据来源：全部 / 最近 / 收藏 */
    fun switchSource(source: FileSource) {
        if (_state.value.source == source) return
        _state.update {
            it.copy(
                source = source,
                breadcrumb = if (source == FileSource.ALL) it.breadcrumb.take(1) else listOf(Breadcrumb(0, labelOf(source))),
            )
        }
        load(0, refreshing = false)
    }

    private fun labelOf(source: FileSource): String = when (source) {
        FileSource.ALL -> "全部文件"
        FileSource.RECENT -> "最近文件"
        FileSource.FAVORITE -> "我的收藏"
    }

    fun enterFolder(item: FileItem) {
        if (!item.isFolder || _state.value.source != FileSource.ALL) return
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

    /** 收藏/取消收藏 */
    fun toggleFavorite(file: FileItem) {
        viewModelScope.launch {
            val isFav = _state.value.favoriteIds.contains(file.id)
            try {
                if (isFav) {
                    safeApi { container.api.unfavorite(mapOf("file_id" to file.id)) }
                } else {
                    safeApi { container.api.favorite(mapOf("file_id" to file.id)) }
                }
                _state.update { s ->
                    val ids = if (isFav) s.favoriteIds - file.id else s.favoriteIds + file.id
                    s.copy(
                        favoriteIds = ids,
                        snackbar = if (isFav) "已取消收藏「${file.name}」" else "已收藏「${file.name}」",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            }
        }
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

    /** 音视频：缓存到本地并调用系统播放器 */
    fun play(file: FileItem) {
        viewModelScope.launch {
            try {
                val msg = DownloadUtil.play(appContext, container.api, file.id, file.name)
                _state.update { it.copy(snackbar = msg) }
            } catch (e: Exception) {
                val msg = (e as? ApiException)?.message ?: "播放失败：${e.message}"
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
