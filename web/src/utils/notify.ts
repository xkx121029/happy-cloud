import { createDiscreteApi } from 'naive-ui'

/** 供非组件环境（axios 拦截器等）使用的消息 / 对话框 */
export const { message, dialog } = createDiscreteApi(['message', 'dialog'])
