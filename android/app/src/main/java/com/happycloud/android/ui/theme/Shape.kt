package com.happycloud.android.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Shapes
import androidx.compose.ui.unit.dp

/** Material 3 Expressive：大圆角、丰富的形状层次 */
val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(10.dp),
    small = RoundedCornerShape(14.dp),
    medium = RoundedCornerShape(20.dp),
    large = RoundedCornerShape(26.dp),
    extraLarge = RoundedCornerShape(32.dp),
)

/** 按钮、输入框等控件使用的统一大圆角 */
val ButtonShape = RoundedCornerShape(18.dp)

/** FAB 形状（非圆形，Expressive 大圆角） */
val FabShape = RoundedCornerShape(20.dp)
