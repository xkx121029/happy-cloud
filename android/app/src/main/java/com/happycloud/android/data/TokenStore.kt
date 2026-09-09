package com.happycloud.android.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.dataStore by preferencesDataStore(name = "happycloud")

/** 登录态存储（DataStore），并提供内存中的 JWT 供拦截器使用 */
class TokenStore(private val context: Context) {

    companion object {
        private val KEY_TOKEN = stringPreferencesKey("token")
        private val KEY_USERNAME = stringPreferencesKey("username")
    }

    /** 内存缓存，TokenInterceptor 同步读取 */
    @Volatile
    var cachedToken: String? = null

    val tokenFlow: Flow<String?> = context.dataStore.data.map { it[KEY_TOKEN] }
    val usernameFlow: Flow<String?> = context.dataStore.data.map { it[KEY_USERNAME] }

    suspend fun getToken(): String? = context.dataStore.data.first()[KEY_TOKEN]

    suspend fun getUsername(): String? = context.dataStore.data.first()[KEY_USERNAME]

    suspend fun save(token: String, username: String) {
        cachedToken = token
        context.dataStore.edit {
            it[KEY_TOKEN] = token
            it[KEY_USERNAME] = username
        }
    }

    suspend fun clear() {
        cachedToken = null
        context.dataStore.edit { it.clear() }
    }
}
