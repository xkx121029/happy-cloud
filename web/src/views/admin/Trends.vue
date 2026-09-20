<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import * as echarts from 'echarts'
import { fetchTrends } from '@/api/admin'
import type { TrendData } from '@/api/types'
import { formatSize } from '@/utils/format'

const loading = ref(false)
const days = ref(30)
const data = ref<TrendData | null>(null)
const activityChartRef = ref<HTMLDivElement>()
const storageChartRef = ref<HTMLDivElement>()
const activityChart = shallowRef<echarts.ECharts | null>(null)
const storageChart = shallowRef<echarts.ECharts | null>(null)

function renderCharts() {
  if (!data.value) return
  const d = data.value

  // 活动趋势：每日注册 + 每日上传（柱状 + 折线）
  if (activityChartRef.value) {
    const chart = activityChart.value ?? echarts.init(activityChartRef.value)
    chart.setOption({
      tooltip: { trigger: 'axis' },
      legend: { data: ['新增用户', '上传文件'], top: 0 },
      grid: { left: 40, right: 20, top: 36, bottom: 28 },
      xAxis: { type: 'category', data: d.days, boundaryGap: false },
      yAxis: { type: 'value', minInterval: 1 },
      series: [
        { name: '新增用户', type: 'bar', data: d.registrations, itemStyle: { borderRadius: [4, 4, 0, 0] } },
        { name: '上传文件', type: 'line', smooth: true, data: d.uploads, showSymbol: false }
      ]
    })
    activityChart.value = chart
  }

  // 存储趋势：每日上传字节量
  if (storageChartRef.value) {
    const chart = storageChart.value ?? echarts.init(storageChartRef.value)
    chart.setOption({
      tooltip: {
        trigger: 'axis',
        valueFormatter: (v: number) => formatSize(Number(v))
      },
      grid: { left: 70, right: 20, top: 20, bottom: 28 },
      xAxis: { type: 'category', data: d.days, boundaryGap: false },
      yAxis: {
        type: 'value',
        axisLabel: { formatter: (v: number) => formatSize(Number(v)) }
      },
      series: [
        {
          name: '当日上传量',
          type: 'bar',
          data: d.upload_bytes,
          areaStyle: { opacity: 0.25 },
          itemStyle: { borderRadius: [4, 4, 0, 0] }
        }
      ]
    })
    storageChart.value = chart
  }
}

async function load() {
  loading.value = true
  try {
    data.value = await fetchTrends(days.value)
    await nextTick()
    renderCharts()
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function onDaysChange() {
  load()
}

function onResize() {
  activityChart.value?.resize()
  storageChart.value?.resize()
}

onMounted(() => {
  load()
  window.addEventListener('resize', onResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  activityChart.value?.dispose()
  storageChart.value?.dispose()
})
</script>

<template>
  <div>
    <div class="page-toolbar">
      <div class="page-title">数据概览</div>
      <div class="toolbar-right">
        <n-select v-model:value="days" style="width: 130px" :options="[
          { label: '近 7 天', value: 7 },
          { label: '近 30 天', value: 30 },
          { label: '近 90 天', value: 90 }
        ]" @update:value="onDaysChange" />
        <n-button :loading="loading" @click="load">刷新</n-button>
      </div>
    </div>

    <n-spin :show="loading">
      <div class="chart-card">
        <div class="chart-title">注册与上传趋势</div>
        <div ref="activityChartRef" class="chart" />
      </div>
      <div class="chart-card">
        <div class="chart-title">每日上传存储量</div>
        <div ref="storageChartRef" class="chart" />
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.chart-card {
  background: var(--card-color, #fff);
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 10px;
  padding: 16px;
  margin-bottom: 12px;
}
.chart-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 8px;
}
.chart {
  height: 320px;
}
</style>
