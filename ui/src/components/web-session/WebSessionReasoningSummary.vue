<script setup lang="ts">
defineProps<{
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
    <div v-if="expanded" class="reasoning-summary-body"><slot /></div>
  </div>
</template>

<style scoped>
.reasoning-summary {
  min-width: 0;
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
  padding: 8px 0 0 16px;
  overflow-wrap: anywhere;
}
</style>
