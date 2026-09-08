<template>
  <div class="chrome-extension-landing">
    <IntegrationLandingLayout
      :title="$t('integrations.chrome.title')"
      :subtitle="$t('integrations.chrome.subtitle')"
      variant="chrome"
    >
      <template #tags>
        <span v-for="key in scenarioKeys" :key="key" class="scenario-tag">
          {{ $t(`integrations.chrome.scenarios.${key}`) }}
        </span>
      </template>

      <template #actions>
        <IntegrationExternalCta
          variant="chrome"
          :label="$t('integrations.chrome.installCta')"
          :hint="$t('integrations.chrome.installCtaHint')"
          @click="openChromeStore"
        >
          <template #icon>
            <t-icon name="extension" size="18px" />
          </template>
        </IntegrationExternalCta>
      </template>

      <template #main>
        <div class="landing-group">
          <!-- 生成配置 Key 面板 -->
          <section class="chrome-plugin-key-section">
            <h4 class="chrome-plugin-key-title">
              {{ $t('integrations.chrome.pluginKeyTitle') }}
            </h4>
            <p class="chrome-plugin-key-desc">{{ $t('integrations.chrome.pluginKeyDesc') }}</p>

            <div v-if="keyLoading" class="key-loading">
              <t-loading size="small" />
              <span>{{ $t('integrations.chrome.loading') }}</span>
            </div>

            <template v-else>
              <!-- 未生成 Key：显示生成按钮 -->
              <div v-if="!chromePluginKey" class="key-generate">
                <t-button theme="primary" size="large" :loading="keyGenerating" @click="generateKey">
                  <template #icon><t-icon name="add" /></template>
                  {{ $t('integrations.chrome.generateKey') }}
                </t-button>
                <p class="key-generate-hint">{{ $t('integrations.chrome.generateKeyHint') }}</p>
              </div>

              <!-- 已生成 Key：显示 Key 信息 -->
              <div v-else class="key-display">
                <div class="key-display-row">
                  <label>{{ $t('integrations.chrome.apiKeyLabel') }}</label>
                  <div class="key-display-control">
                    <t-input :model-value="chromePluginKey.api_key" readonly class="mono-text-input" />
                    <t-button
                      size="small"
                      variant="text"
                      :title="$t('integrations.chrome.copy')"
                      @click="copyApiKey"
                    >
                      <t-icon name="file-copy" size="16px" />
                    </t-button>
                  </div>
                </div>
                <div class="key-display-row">
                  <label>{{ $t('integrations.chrome.serviceUrlLabel') }}</label>
                  <div class="key-display-control">
                    <t-input :model-value="apiBaseUrlDisplay" readonly class="mono-text-input" />
                    <t-button
                      size="small"
                      variant="text"
                      :title="$t('integrations.chrome.copy')"
                      @click="copyApiUrl"
                    >
                      <t-icon name="file-copy" size="16px" />
                    </t-button>
                  </div>
                </div>
                <div class="key-display-actions">
                  <t-button theme="danger" variant="outline" :loading="keyDeleting" @click="confirmDeleteKey">
                    <template #icon><t-icon name="delete" /></template>
                    {{ $t('integrations.chrome.deleteKey') }}
                  </t-button>
                </div>
              </div>
            </template>
          </section>
        </div>
      </template>

      <template #aside>
        <div class="landing-group">
          <section class="setting-drawer__section">
            <h4 class="setting-drawer__section-title">{{ $t('integrations.chrome.stepsTitle') }}</h4>
            <ol class="landing-steps">
              <li v-for="(step, index) in stepKeys" :key="step" class="landing-step">
                <span class="landing-step-num">{{ index + 1 }}</span>
                <div class="landing-step-body">
                  <div class="landing-step-title">{{ $t(`integrations.chrome.steps.${step}.title`) }}</div>
                  <p class="landing-step-desc">{{ $t(`integrations.chrome.steps.${step}.desc`) }}</p>

                  <!-- 安装步骤：显示 Chrome 商店链接 -->
                  <t-button
                    v-if="step === 'install'"
                    size="small"
                    variant="outline"
                    class="landing-step-action"
                    @click="openChromeStore"
                  >
                    <template #icon><t-icon name="extension" size="14px" /></template>
                    {{ $t('integrations.chrome.goToStore') }}
                  </t-button>

                  <!-- 连接步骤：显示教程图片 -->
                  <div v-if="step === 'connect'" class="landing-step-tutorial-images">
                    <div class="tutorial-image" @click="showImagePreview(0)">
                      <img :src="tutorialImage1" :alt="$t('integrations.chrome.tutorialImage1')" />
                      <span class="tutorial-image-hint">{{ $t('integrations.chrome.clickToEnlarge') }}</span>
                    </div>
                    <div class="tutorial-image" @click="showImagePreview(1)">
                      <img :src="tutorialImage2" :alt="$t('integrations.chrome.tutorialImage2')" />
                      <span class="tutorial-image-hint">{{ $t('integrations.chrome.clickToEnlarge') }}</span>
                    </div>
                  </div>
                </div>
              </li>
            </ol>
          </section>
        </div>
      </template>

      <template #footer>
        <span class="landing-meta">{{ $t('integrations.chrome.storeMeta') }}</span>
      </template>
    </IntegrationLandingLayout>

    <!-- 图片预览弹窗 -->
    <t-dialog
      v-model:visible="imagePreviewVisible"
      :header="$t('integrations.chrome.tutorialPreview')"
      :footer="false"
      width="720px"
    >
      <img v-if="currentPreviewImage" :src="currentPreviewImage" class="preview-image" />
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { copyWithToast } from '@/utils/clipboard'
import { CHROME_EXTENSION_URL } from '@/config/integrations'
import { useApiBaseUrlDisplay } from '@/composables/useApiBaseUrlDisplay'
import { useI18n } from 'vue-i18n'
import {
  deleteChromePluginKey,
  generateChromePluginKey,
  getChromePluginKey,
  type ChromePluginKey,
} from '@/api/chrome-plugin-key'
import IntegrationLandingLayout from './IntegrationLandingLayout.vue'
import IntegrationExternalCta from './IntegrationExternalCta.vue'

// 教程图片 - 从 assets 导入
const tutorialImage1 = new URL('@/assets/images/chrome-tutorial-1.png', import.meta.url).href
const tutorialImage2 = new URL('@/assets/images/chrome-tutorial-2.png', import.meta.url).href

const { t } = useI18n()
const { apiBaseUrlDisplay } = useApiBaseUrlDisplay()

const capabilityKeys = ['qa', 'clip', 'notes', 'shortcuts'] as const
const scenarioKeys = ['research', 'learning', 'tech', 'work'] as const
// 简化步骤：只保留安装和连接
const stepKeys = ['install', 'connect'] as const

const capabilityIcons: Record<(typeof capabilityKeys)[number], string> = {
  qa: 'chat-bubble',
  clip: 'file-copy',
  notes: 'edit',
  shortcuts: 'jump',
}

// Chrome plugin key state
const chromePluginKey = ref<ChromePluginKey | null>(null)
const keyLoading = ref(false)
const keyGenerating = ref(false)
const keyDeleting = ref(false)

// Image preview state
const imagePreviewVisible = ref(false)
const currentPreviewImage = ref('')

function showImagePreview(index: number) {
  currentPreviewImage.value = index === 0 ? tutorialImage1 : tutorialImage2
  imagePreviewVisible.value = true
}

const openChromeStore = () => {
  window.open(CHROME_EXTENSION_URL, '_blank', 'noopener,noreferrer')
}

async function loadChromePluginKey() {
  keyLoading.value = true
  try {
    const resp = await getChromePluginKey()
    chromePluginKey.value = resp.data
  } catch (e: any) {
    console.error('Failed to load Chrome plugin key:', e)
  } finally {
    keyLoading.value = false
  }
}

async function generateKey() {
  keyGenerating.value = true
  try {
    const resp = await generateChromePluginKey()
    chromePluginKey.value = resp.data
    MessagePlugin.success(t('integrations.chrome.keyGenerated'))
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('integrations.chrome.generateKeyFailed'))
  } finally {
    keyGenerating.value = false
  }
}

function confirmDeleteKey() {
  const dialog = DialogPlugin.confirm({
    header: t('integrations.chrome.deleteKeyTitle'),
    body: t('integrations.chrome.deleteKeyConfirm'),
    onConfirm: async () => {
      dialog.destroy()
      keyDeleting.value = true
      try {
        await deleteChromePluginKey()
        chromePluginKey.value = null
        MessagePlugin.success(t('integrations.chrome.keyDeleted'))
      } catch (e: any) {
        MessagePlugin.error(e?.message || t('integrations.chrome.deleteKeyFailed'))
      } finally {
        keyDeleting.value = false
      }
    },
    onCancel: () => dialog.destroy(),
  })
}

async function copyApiKey() {
  if (!chromePluginKey.value) return
  await copyWithToast(chromePluginKey.value.api_key, 'integrations.chrome.copySuccess')
}

async function copyApiUrl() {
  await copyWithToast(apiBaseUrlDisplay.value, 'integrations.chrome.copySuccess')
}

onMounted(() => {
  void loadChromePluginKey()
})
</script>

<style scoped lang="less">
.chrome-extension-landing {
  width: 100%;
}

// 生成配置 Key 面板
.chrome-plugin-key-section {
  margin-top: 24px;
  padding: 0;

  .chrome-plugin-key-title {
    margin: 0 0 8px;
    font-size: 13px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    line-height: 1.4;
  }

  .chrome-plugin-key-desc {
    margin: 0 0 20px;
    font-size: 12px;
    color: var(--td-text-color-secondary);
    line-height: 1.5;
  }
}

.key-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 32px 0;
  color: var(--td-text-color-secondary);
  font-size: 14px;
}

.key-generate {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 24px 0;

  .key-generate-hint {
    margin-top: 16px;
    font-size: 13px;
    color: var(--td-text-color-placeholder);
    line-height: 1.5;
    text-align: center;
  }
}

.key-display {
  .key-display-row {
    margin-bottom: 20px;

    label {
      display: block;
      margin-bottom: 8px;
      font-size: 13px;
      font-weight: 500;
      color: var(--td-text-color-primary);
    }
  }

  .key-display-control {
    display: flex;
    align-items: center;
    gap: 12px;

    .mono-text-input {
      flex: 1;
      font-family: 'SF Mono', Monaco, Consolas, 'Courier New', monospace;
      font-size: 13px;
    }
  }

  .key-display-actions {
    margin-top: 28px;
    padding-top: 20px;
    border-top: 1px solid var(--td-component-stroke);
    display: flex;
    justify-content: flex-end;
  }
}

.landing-step-tutorial-images {
  display: flex;
  gap: 12px;
  margin-top: 12px;

  .tutorial-image {
    position: relative;
    flex: 1;
    cursor: pointer;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid var(--td-component-stroke);
    transition: border-color 0.2s, box-shadow 0.2s;

    &:hover {
      border-color: var(--td-brand-color);
      box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);

      .tutorial-image-hint {
        opacity: 1;
      }
    }

    img {
      width: 100%;
      height: auto;
      display: block;
    }

    .tutorial-image-hint {
      position: absolute;
      bottom: 0;
      left: 0;
      right: 0;
      padding: 6px 8px;
      background: rgba(0, 0, 0, 0.6);
      color: white;
      font-size: 11px;
      text-align: center;
      opacity: 0;
      transition: opacity 0.2s;
    }
  }
}

.preview-image {
  width: 100%;
  height: auto;
  display: block;
  border-radius: 4px;
}
</style>