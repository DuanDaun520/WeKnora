<template>
  <div class="usage-stats-settings">
    <div class="section-header">
      <h2>{{ t('usageStats.title') }}</h2>
      <p class="section-description">{{ t('usageStats.personalDesc') }}</p>
    </div>
    <UsageSummarySection :fetcher="myFetcher" :group-by-options="MY_GROUP_BYS" />
  </div>
</template>

<script setup lang="ts">
// 个人 AI 使用统计（docs/Token统计与计费设计.md §5.1）：设置页的
// 「个人用量」入口，所有用户可见，仅统计当前空间内本人的调用。
// 身份由后端从认证上下文取，前端不传 user_id。
import { useI18n } from 'vue-i18n'
import UsageSummarySection from './usage/UsageSummarySection.vue'
import { getMyUsageSummary, type UsageGroupBy, type UsageQueryParams } from '@/api/usage'

const { t } = useI18n()

// 个人视角的聚合维度：类型 / 模型 / 按天（天维度后端附带模型细分）。
const MY_GROUP_BYS: UsageGroupBy[] = ['category', 'model', 'day']

// setup 只跑一次，普通函数即稳定引用，不会误触发子组件的 watch(fetcher)。
const myFetcher = (params: UsageQueryParams) => getMyUsageSummary(params)
</script>

<style scoped>
.usage-stats-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
