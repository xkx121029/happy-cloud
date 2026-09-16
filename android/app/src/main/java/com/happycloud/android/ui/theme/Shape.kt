package com.happycloud.android.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Shapes
import androidx.compose.ui.unit.dp

/** Material 3 Expressive：收敛圆角，偏"接近直角+轻微弧"，更克制 */
val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(16.dp),
    large = RoundedCornerShape(20.dp),
    extraLarge = RoundedCornerShape(24.dp),
)

/** 按钮、输入框等控件使用的统一圆角 */
val ButtonShape = RoundedCornerShape(14.dp)

/** FAB 形状（非圆形，克制的 16dp） */
val FabShape = RoundedCornerShape(16.dp)

/** 对话框统一样式圆角 */
val DialogShape = RoundedCornerShape(22.dp)
