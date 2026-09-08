<template>
  <t-dialog
    :visible="visible"
    :header="t('userProfile.avatar.cropTitle')"
    :footer="false"
    width="420px"
    :close-on-overlay-click="false"
    @close="close"
    @closed="reset"
  >
    <div class="crop-body">
      <div
        ref="stageEl"
        class="crop-stage"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="endPointerDrag"
        @pointercancel="endPointerDrag"
        @wheel.prevent="onWheel"
      >
        <canvas ref="displayCanvasEl" class="crop-image" :style="imageStyle" />
        <!-- 圆形取景框：巨型 box-shadow 把圆外压暗，圆环高亮边界 -->
        <div class="crop-mask" />
        <div v-if="loadFailed" class="crop-error">{{ t('userProfile.avatar.invalidType') }}</div>
      </div>
      <div class="crop-hint">{{ t('userProfile.avatar.cropHint') }}</div>
      <div class="crop-zoom">
        <span class="crop-zoom__icon">－</span>
        <t-slider
          :value="scale"
          :min="minScale"
          :max="maxScale"
          :step="0.001"
          :label="false"
          @change="onSliderChange"
        />
        <span class="crop-zoom__icon">＋</span>
      </div>
      <div class="crop-footer">
        <t-button variant="outline" :disabled="exporting" @click="close">
          {{ t('common.cancel') }}
        </t-button>
        <t-button
          theme="primary"
          :loading="exporting"
          :disabled="!ready || loadFailed"
          @click="confirmCrop"
        >
          {{ t('userProfile.avatar.save') }}
        </t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
/**
 * 手写头像裁剪器（零依赖）：拖动平移 + 滚轮/滑杆缩放 + 圆形取景框，
 * 导出 512×512 JPEG（圆形由展示端 border-radius 呈现，导出保持方形）。
 *
 * EXIF 方向：优先 createImageBitmap(file, { imageOrientation: 'from-image' })
 * 烘焙方向；不支持该选项的浏览器回落 HTMLImageElement（现代浏览器对
 * <img> 默认应用 from-image，drawImage 取到的是已矫正的位图）。
 */
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  visible: boolean
  file: File | null
}>()

const emit = defineEmits<{
  (e: 'confirm', file: File): void
  (e: 'update:visible', value: boolean): void
  (e: 'cancel'): void
}>()

const { t } = useI18n()

/** 取景框边长（显示像素）。 */
const VIEWPORT = 320
/** 导出尺寸（方形 JPEG）。 */
const EXPORT_SIZE = 512
const MAX_ZOOM = 4

const stageEl = ref<HTMLElement | null>(null)
const displayCanvasEl = ref<HTMLCanvasElement | null>(null)

const srcW = ref(0)
const srcH = ref(0)
const scale = ref(1)
const offsetX = ref(0)
const offsetY = ref(0)
const ready = ref(false)
const loadFailed = ref(false)
const exporting = ref(false)

const minScale = computed(() =>
  (srcW.value && srcH.value)
    ? Math.max(VIEWPORT / srcW.value, VIEWPORT / srcH.value)
    : 1,
)
const maxScale = computed(() => minScale.value * MAX_ZOOM)

const imageStyle = computed(() => ({
  width: `${srcW.value}px`,
  height: `${srcH.value}px`,
  transform: `translate(calc(-50% + ${offsetX.value}px), calc(-50% + ${offsetY.value}px)) scale(${scale.value})`,
}))

// 平移夹紧：圆内始终被图覆盖（|offset| ≤ (缩放后边长 − 视口)/2）。
function clampOffsets(): void {
  const maxX = Math.max(0, (srcW.value * scale.value - VIEWPORT) / 2)
  const maxY = Math.max(0, (srcH.value * scale.value - VIEWPORT) / 2)
  offsetX.value = Math.min(maxX, Math.max(-maxX, offsetX.value))
  offsetY.value = Math.min(maxY, Math.max(-maxY, offsetY.value))
}

// 以视口中心为锚缩放：视口中心对应的图像点保持不动。
function zoomTo(next: number): void {
  const clamped = Math.min(maxScale.value, Math.max(minScale.value, next))
  const ratio = clamped / scale.value
  scale.value = clamped
  offsetX.value *= ratio
  offsetY.value *= ratio
  clampOffsets()
}

let objectURL = ''
let dragging = false
let dragStartX = 0
let dragStartY = 0
let dragBaseX = 0
let dragBaseY = 0

function onPointerDown(e: PointerEvent): void {
  if (!ready.value) return
  dragging = true
  dragStartX = e.clientX
  dragStartY = e.clientY
  dragBaseX = offsetX.value
  dragBaseY = offsetY.value
  stageEl.value?.setPointerCapture(e.pointerId)
}

function onPointerMove(e: PointerEvent): void {
  if (!dragging) return
  offsetX.value = dragBaseX + (e.clientX - dragStartX)
  offsetY.value = dragBaseY + (e.clientY - dragStartY)
  clampOffsets()
}

function endPointerDrag(e: PointerEvent): void {
  if (!dragging) return
  dragging = false
  try { stageEl.value?.releasePointerCapture(e.pointerId) } catch { /* 已释放 */ }
}

function onWheel(e: WheelEvent): void {
  if (!ready.value) return
  zoomTo(scale.value * (e.deltaY < 0 ? 1.08 : 1 / 1.08))
}

function onSliderChange(value: number | number[]): void {
  const next = Array.isArray(value) ? value[0] : value
  if (typeof next === 'number' && Number.isFinite(next)) zoomTo(next)
}

async function loadSource(): Promise<void> {
  const file = props.file
  if (!file) return
  loadFailed.value = false
  ready.value = false
  try {
    let source: CanvasImageSource
    let w = 0
    let h = 0
    try {
      const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
      source = bitmap
      w = bitmap.width
      h = bitmap.height
    } catch {
      // 回退：现代浏览器对 <img> 默认应用 EXIF 方向，drawImage 拿到的
      // 是矫正后的位图（不支持的旧浏览器会裁歪竖拍照片，已知限制）。
      objectURL = URL.createObjectURL(file)
      const img = await new Promise<HTMLImageElement>((resolve, reject) => {
        const el = new Image()
        el.onload = () => resolve(el)
        el.onerror = () => reject(new Error('image decode failed'))
        el.src = objectURL
      })
      source = img
      w = img.naturalWidth
      h = img.naturalHeight
    }
    if (!w || !h) throw new Error('empty image')

    await nextTick()
    const canvas = displayCanvasEl.value
    if (!canvas) return
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('canvas unavailable')
    ctx.drawImage(source, 0, 0)
    srcW.value = w
    srcH.value = h
    scale.value = minScale.value
    offsetX.value = 0
    offsetY.value = 0
    ready.value = true
  } catch {
    loadFailed.value = true
  }
}

watch(
  () => [props.visible, props.file] as const,
  ([visible, file]) => {
    if (visible && file) void loadSource()
  },
  { immediate: true },
)

function releaseObjectURL(): void {
  if (objectURL) {
    URL.revokeObjectURL(objectURL)
    objectURL = ''
  }
}

function reset(): void {
  releaseObjectURL()
  srcW.value = 0
  srcH.value = 0
  scale.value = 1
  offsetX.value = 0
  offsetY.value = 0
  ready.value = false
  loadFailed.value = false
}

function close(): void {
  emit('update:visible', false)
  emit('cancel')
}

function toBlob(canvas: HTMLCanvasElement): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, 'image/jpeg', 0.92))
}

async function confirmCrop(): Promise<void> {
  if (!ready.value || exporting.value) return
  exporting.value = true
  try {
    // 显示坐标 → 图像坐标：可视窗口在原图上的采样矩形。
    const sample = VIEWPORT / scale.value
    const sx = (srcW.value / 2) - (offsetX.value / scale.value) - (sample / 2)
    const sy = (srcH.value / 2) - (offsetY.value / scale.value) - (sample / 2)

    const out = document.createElement('canvas')
    out.width = EXPORT_SIZE
    out.height = EXPORT_SIZE
    const ctx = out.getContext('2d')
    if (!ctx || !displayCanvasEl.value) throw new Error('canvas unavailable')
    // JPEG 无 alpha：先铺白底，避免透明 PNG 裁出黑块。
    ctx.fillStyle = '#ffffff'
    ctx.fillRect(0, 0, EXPORT_SIZE, EXPORT_SIZE)
    ctx.drawImage(
      displayCanvasEl.value, sx, sy, sample, sample,
      0, 0, EXPORT_SIZE, EXPORT_SIZE,
    )

    const blob = await toBlob(out)
    if (!blob) throw new Error('encode failed')
    emit('confirm', new File([blob], 'avatar.jpg', { type: 'image/jpeg' }))
    emit('update:visible', false)
  } finally {
    exporting.value = false
  }
}
</script>

<style scoped lang="less">
.crop-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.crop-stage {
  position: relative;
  width: 320px;
  height: 320px;
  margin: 0 auto;
  overflow: hidden;
  border-radius: var(--td-radius-medium);
  background: var(--td-gray-color-14);
  cursor: grab;
  touch-action: none; /* 拖动平移时拦截移动端滚动 */
  user-select: none;

  &:active {
    cursor: grabbing;
  }
}

.crop-image {
  position: absolute;
  left: 50%;
  top: 50%;
  transform-origin: center center;
  max-width: none; /* 覆写 canvas 默认收缩，尺寸完全由 style 控制 */
  will-change: transform;
}

.crop-mask {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  border: 2px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.55);
  pointer-events: none;
}

.crop-error {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
  text-align: center;
}

.crop-hint {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  text-align: center;
}

.crop-zoom {
  display: flex;
  align-items: center;
  gap: 8px;

  &__icon {
    color: var(--td-text-color-secondary);
    flex-shrink: 0;
    font-size: 12px;
  }
}

.crop-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
