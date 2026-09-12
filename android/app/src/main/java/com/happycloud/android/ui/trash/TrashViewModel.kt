package com.happycloud.android.ui.trash

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.happycloud.android.AppContainer
import com.happycloud.android.data.ApiException
import com.happycloud.android.data.FileItem
import com.happycloud.android.data.safeApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 回收站数据逻辑 */
class TrashViewModel(private val container: AppContainer) : ViewModel() {

    data class UiState(
        val items: List<FileItem> = emptyList(),
        val loading: Boolean = false,
        val refreshing: Boolean = false,
        val error: String? = null,
        val snackbar: String? = null,
    )

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state.asStateFlow()

    init {
        load()
    }

    fun refresh() = load(refreshing = true)

    private fun load(refreshing: Boolean = false) {
        viewModelScope.launch {
            _state.update {
                if (refreshing) it.copy(refreshing = true, error = null)
                else it.copy(loading = true, error = null)
            }
            try {
                val data = safeApi { container.api.trash() }
                _state.update {
                    it.copy(items = data.items, loading = false, refreshing = false, error = null)
                }
            } catch (e: ApiException) {
                _state.update { it.copy(loading = false, refreshing = false, error = e.message) }
            }
        }
    }

    /** 恢复单个文件 */
    fun restore(file: FileItem, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi { container.api.restore(mapOf("file_ids" to listOf(file.id))) }
                refresh()
                _state.update { it.copy(snackbar = "已恢复「${file.name}」") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    /** 彻底删除单个文件 */
    fun deletePermanent(file: FileItem, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi { container.api.deletePermanent(mapOf("file_ids" to listOf(file.id))) }
                refresh()
                _state.update { it.copy(snackbar = "已彻底删除「${file.name}」") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    /** 清空回收站 */
    fun clearAll(onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi { container.api.clearTrash() }
                refresh()
                _state.update { it.copy(snackbar = "回收站已清空") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    fun consumeSnackbar() = _state.update { it.copy(snackbar = null) }
}

fun trashViewModelFactory(container: AppContainer): ViewModelProvider.Factory = viewModelFactory {
    initializer {
        TrashViewModel(container)
    }
}
