<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue';

const props = defineProps<{
  text: string;
  label: string;
  time: string;
  timeTitle: string;
  expanded?: boolean;
  /** Latest thought line, so a collapsed header reads like a live ticker. */
  summary?: string;
  streaming?: boolean;
}>();

defineEmits<{ toggle: [] }>();

const bodyRef = ref<HTMLElement | null>(null);
const contentRef = ref<HTMLElement | null>(null);

/**
 * The body grows with its content up to the height cap, then keeps streaming
 * inside the fixed frame. While streaming it stays pinned to the newest text
 * unless the reader scrolls up to look back.
 */
const BODY_PIN_THRESHOLD_PX = 32;
let pinnedToBottom = true;
let contentObserver: ResizeObserver | null = null;

function isNearBottom(element: HTMLElement) {
  return element.scrollHeight - element.scrollTop - element.clientHeight <= BODY_PIN_THRESHOLD_PX;
}

function syncPinnedScroll() {
  const body = bodyRef.value;
  if (!body || !props.streaming || !pinnedToBottom) {
    return;
  }
  body.scrollTop = body.scrollHeight;
}

function handleBodyScroll() {
  const body = bodyRef.value;
  if (body && props.streaming) {
    pinnedToBottom = isNearBottom(body);
  }
}

function stopContentObservation() {
  contentObserver?.disconnect();
  contentObserver = null;
}

function startContentObservation() {
  const content = contentRef.value;
  if (!content || contentObserver || typeof ResizeObserver === 'undefined') {
    return;
  }
  // The frame is height-capped, so its own size stops changing once the cap is
  // reached; observing the content catches every growth from either source.
  contentObserver = new ResizeObserver(() => syncPinnedScroll());
  contentObserver.observe(content);
}

watch(
  () => [props.expanded, props.streaming] as const,
  ([expanded, streaming], previous) => {
    if (!expanded) {
      stopContentObservation();
      return;
    }
    const wasExpanded = previous?.[0] === true;
    const wasStreaming = previous?.[1] === true;
    // The immediate first call acts as a fresh open, so a body that mounts
    // already-expanded attaches observation and pins straight away.
    if (!wasExpanded) {
      // Freshly opened: settled thoughts read from the top, a live stream
      // opens on its tail and keeps following the output.
      pinnedToBottom = streaming === true;
      void nextTick(() => {
        startContentObservation();
        syncPinnedScroll();
      });
      return;
    }
    if (wasStreaming && !streaming) {
      // Segment settled: stop chasing the tail and keep the reading position.
      pinnedToBottom = false;
    }
  },
  { immediate: true, flush: 'post' }
);

// ResizeObserver covers environments where it exists; the text watch keeps the
// pin working where it does not (and after throttled stream swaps).
watch(
  () => props.text,
  () => {
    void nextTick(syncPinnedScroll);
  }
);

onBeforeUnmount(stopContentObservation);
</script>

<template>
  <div v-if="text.trim()" class="reasoning-summary">
    <button
      type="button"
      class="reasoning-summary-toggle"
      :aria-expanded="Boolean(expanded)"
      @click="$emit('toggle')"
    >
      <span aria-hidden="true">{{ expanded ? '▾' : '▸' }}</span>
      <span class="reasoning-summary-label" :class="{ 'is-streaming': streaming }">
        <span v-if="streaming" class="reasoning-summary-dot" aria-hidden="true"></span>
        {{ label }}
      </span>
      <span v-if="summary" class="reasoning-summary-preview">{{ summary }}</span>
      <span class="reasoning-summary-time" :title="timeTitle">{{ time }}</span>
    </button>
    <div
      v-if="expanded"
      ref="bodyRef"
      class="reasoning-summary-body"
      @scroll.passive="handleBodyScroll"
    >
      <div ref="contentRef" class="reasoning-summary-body-content"><slot /></div>
    </div>
  </div>
</template>

<style scoped>
.reasoning-summary {
  /* The timeline item is a column flex box with `align-items: flex-start`,
     so without this clamp a wide inline `pre`/code run would size the whole
     disclosure to its content and stretch the timeline horizontally. */
  min-width: 0;
  max-width: 100%;
}

.reasoning-summary-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 100%;
  padding: 4px 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.reasoning-summary-label {
  flex: 0 0 auto;
}

.reasoning-summary-label.is-streaming {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.reasoning-summary-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.75;
  animation: livePulse 1.4s ease-in-out infinite;
}

.reasoning-summary-preview {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  opacity: 0.6;
}

.reasoning-summary-time {
  flex: 0 0 auto;
  font-size: 11px;
  opacity: 0.65;
}

.reasoning-summary-body {
  max-height: 260px;
  /* Wide `pre` runs scroll inside the frame instead of stretching the column. */
  overflow: auto;
  max-width: 100%;
  margin: 6px 0 2px 16px;
  padding: 8px 12px;
  border: 1px solid color-mix(in srgb, var(--n-border-color) 78%, transparent);
  border-radius: 6px;
  background: var(
    --app-surface-sunken,
    color-mix(in srgb, var(--app-surface-color, #fff) 78%, #111827 6%)
  );
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
  overflow-wrap: anywhere;
}
</style>
