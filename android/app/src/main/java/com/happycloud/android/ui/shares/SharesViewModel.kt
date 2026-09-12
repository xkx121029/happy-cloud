package com.happycloud.android.ui.shares

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.happycloud.android.AppContainer
import com.happycloud.android.data.ApiException
import com.happycloud.android.data.MyShareItem
import com.happycloud.android.data.safeApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.util.Calendar

/** 分享管理数据逻辑 */
class SharesViewModel(private val container: AppContainer) : ViewModel() {

    data class UiState(
        val items: List<MyShareItem> = emptyList(),
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
                val data = safeApi { container.api.myShares() }
                _state.update {
                    it.copy(items = data.items, loading = false, refreshing = false, error = null)
                }
            } catch (e: ApiException) {
                _state.update { it.copy(loading = false, refreshing = false, error = e.message) }
            }
        }
    }

    /** 取消分享 */
    fun cancel(share: MyShareItem, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                safeApi { container.api.cancelShare(share.id) }
                refresh()
                _state.update { it.copy(snackbar = "已取消分享「${share.fileName}」") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    /** 更新有效期：days=null 表示永久（清除过期时间） */
    fun updateExpire(share: MyShareItem, days: Int?, onDone: () -> Unit) {
        viewModelScope.launch {
            try {
                val body = if (days == null) {
                    mapOf<String, Any?>("clear_expire" to true)
                } else {
                    val cal = Calendar.getInstance().apply { add(Calendar.DAY_OF_YEAR, days) }
                    mapOf<String, Any?>("expire_at" to java.text.SimpleDateFormat(
                        "yyyy-MM-dd'T'HH:mm:ssXXX", java.util.Locale.US
                    ).format(cal.time))
                }
                safeApi { container.api.updateShare(share.id, body) }
                refresh()
                val desc = if (days == null) "永久有效" else "有效期 $days 天"
                _state.update { it.copy(snackbar = "已将「${share.fileName}」设为$desc") }
            } catch (e: ApiException) {
                _state.update { it.copy(snackbar = e.message) }
            } finally {
                onDone()
            }
        }
    }

    fun consumeSnackbar() = _state.update { it.copy(snackbar = null) }
}

fun sharesViewModelFactory(container: AppContainer): ViewModelProvider.Factory = viewModelFactory {
    initializer {
        SharesViewModel(container)
    }
}
