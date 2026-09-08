<template>
  <div
    class="user-avatar"
    :class="`user-avatar--${size}`"
    :title="name"
  >
    <img
      v-if="src && !broken"
      class="user-avatar__img"
      :src="src"
      :alt="name || t('common.avatar')"
      @error="broken = true"
    >
    <!-- 灰色默认通用头像：底色 --td-gray-color-2 + 人形剪影，暗色模式自适应 -->
    <svg
      v-else
      class="user-avatar__fallback"
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <circle cx="12" cy="8.2" r="3.6" fill="currentColor" />
      <path d="M4.5 20.4c.9-3.6 3.9-5.6 7.5-5.6s6.6 2 7.5 5.6c.2.8-.4 1.6-1.3 1.6H5.8c-.9 0-1.5-.8-1.3-1.6z" fill="currentColor" />
    </svg>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(defineProps<{
  /** 可加载的图片 URL（blob:）。空/加载失败回落灰色默认。 */
  src?: string | null
  /** 用户名，用于 alt/title 无障碍文案。 */
  name?: string
  size?: 'sm' | 'md' | 'lg'
}>(), {
  src: null,
  name: '',
  size: 'md',
})

const { t } = useI18n()

// 加载失败翻回默认剪影；src 换新图时复位重试。
const broken = ref(false)
watch(() => props.src, () => {
  broken.value = false
})
</script>

<style scoped lang="less">
.user-avatar {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  overflow: hidden;
  flex-shrink: 0;
  background: var(--td-gray-color-2);
  color: var(--td-gray-color-5);

  &.user-avatar--sm {
    width: 24px;
    height: 24px;

    .user-avatar__fallback {
      width: 16px;
      height: 16px;
    }
  }

  &.user-avatar--md {
    .user-avatar__fallback {
      width: 22px;
      height: 22px;
    }
  }

  &.user-avatar--lg {
    width: 64px;
    height: 64px;

    .user-avatar__fallback {
      width: 44px;
      height: 44px;
    }
  }
}

.user-avatar__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.user-avatar__fallback {
  display: block;
}
</style>
