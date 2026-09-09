package com.happycloud.android.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Cloud
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Person
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.happycloud.android.AppContainer
import com.happycloud.android.LocalAppContainer
import com.happycloud.android.ui.files.FilesScreen
import com.happycloud.android.ui.files.FilesViewModel
import com.happycloud.android.ui.files.filesViewModelFactory
import com.happycloud.android.ui.login.LoginScreen
import com.happycloud.android.ui.login.LoginViewModel
import com.happycloud.android.ui.login.loginViewModelFactory
import com.happycloud.android.ui.me.MeScreen
import com.happycloud.android.ui.register.RegisterScreen
import com.happycloud.android.ui.upload.UploadScreen
import kotlinx.coroutines.flow.first

object Routes {
    const val LOGIN = "login"
    const val REGISTER = "register"
    const val MAIN = "main"
}

@Composable
fun AppNavHost() {
    val navController = rememberNavController()
    val container = LocalAppContainer.current
    var startDestination by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) {
        val token = container.tokenStore.tokenFlow.first()
        startDestination = if (token.isNullOrBlank()) Routes.LOGIN else Routes.MAIN
    }

    val destination = startDestination ?: return
    NavHost(navController = navController, startDestination = destination) {
        composable(Routes.LOGIN) {
            val vm: LoginViewModel = viewModel(factory = loginViewModelFactory(container))
            LoginScreen(
                viewModel = vm,
                onLoginSuccess = { navController.navigateToMain() },
                onGoRegister = { navController.navigate(Routes.REGISTER) },
            )
        }
        composable(Routes.REGISTER) {
            RegisterScreen(
                container = container,
                onRegistered = { navController.navigateToMain() },
                onBack = { navController.popBackStack() },
            )
        }
        composable(Routes.MAIN) {
            MainScreen(
                container = container,
                onLogout = { navController.navigateToLogin() },
            )
        }
    }
}

/** 主界面：底部导航（文件 / 上传任务 / 我的） */
@Composable
private fun MainScreen(
    container: AppContainer,
    onLogout: () -> Unit,
) {
    val filesViewModel: FilesViewModel = viewModel(factory = filesViewModelFactory(container))
    var tab by rememberSaveable { mutableStateOf(0) }

    Column(Modifier.fillMaxSize()) {
        Box(Modifier.weight(1f)) {
            when (tab) {
                0 -> FilesScreen(
                    modifier = Modifier.fillMaxSize(),
                    viewModel = filesViewModel,
                    container = container,
                )
                1 -> UploadScreen(
                    modifier = Modifier.fillMaxSize(),
                    container = container,
                    onOpenFiles = { tab = 0 },
                )
                else -> MeScreen(
                    modifier = Modifier.fillMaxSize(),
                    container = container,
                    onLogout = {
                        onLogout()
                    },
                )
            }
        }
        NavigationBar {
            NavigationBarItem(
                selected = tab == 0,
                onClick = { tab = 0 },
                icon = { Icon(Icons.Filled.Cloud, contentDescription = null) },
                label = { Text("文件") },
            )
            NavigationBarItem(
                selected = tab == 1,
                onClick = { tab = 1 },
                icon = { Icon(Icons.Filled.Folder, contentDescription = null) },
                label = { Text("上传") },
            )
            NavigationBarItem(
                selected = tab == 2,
                onClick = { tab = 2 },
                icon = { Icon(Icons.Filled.Person, contentDescription = null) },
                label = { Text("我的") },
            )
        }
    }
}

private fun NavHostController.navigateToMain() {
    navigate(Routes.MAIN) {
        popUpTo(0) { inclusive = true }
    }
}

private fun NavHostController.navigateToLogin() {
    navigate(Routes.LOGIN) {
        popUpTo(0) { inclusive = true }
    }
}
