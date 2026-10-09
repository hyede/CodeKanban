<script setup lang="ts">
import { computed, ref } from 'vue';

const props = defineProps<{
  label: string;
  state: 'request' | 'approve' | 'reject' | 'cancel';
  prompt: string;
  command: string;
  time: string;
  timeTitle: string;
  searchMatch?: boolean;
  searchActive?: boolean;
}>();

const expanded = ref(false);
const summary = computed(() => (props.prompt || props.command).replace(/\s+/g, ' ').trim());
</script>

<template>
  <div class="approval-history" :class="`state-${state}`">
    <button
      type="button"
      class="approval-history-toggle"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <span class="approval-history-chevron" aria-hidden="true">{{ expanded ? '▾' : '▸' }}</span>
      <span
        class="approval-history-label"
        :class="{
          'timeline-search-role-highlight': searchMatch,
          'timeline-search-role-highlight-active': searchActive,
        }"
        >{{ label }}</span
      >
      <span class="approval-history-summary" :title="prompt || command">{{ summary }}</span>
      <span class="approval-history-time" :title="timeTitle">{{ time }}</span>
    </button>
    <div v-if="expanded" class="approval-history-body">
      <div v-if="prompt" class="approval-history-prompt">{{ prompt }}</div>
      <pre v-if="command" class="approval-history-command">{{ command }}</pre>
    </div>
  </div>
</template>

<style scoped>
.approval-history {
  --approval-history-color: var(--web-session-approval-accent-strong, #d97706);
  width: 100%;
  min-width: 0;
  max-width: 100%;
}

.approval-history.state-approve {
  --approval-history-color: var(--app-success, #10b981);
}

.approval-history.state-reject {
  --approval-history-color: var(--app-error, #ef4444);
}

.approval-history.state-cancel {
  --approval-history-color: var(--n-text-color-3);
}

.approval-history-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
  padding: 5px 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  line-height: 20px;
  text-align: left;
  cursor: pointer;
}

.approval-history-toggle:hover {
  background: color-mix(in srgb, var(--approval-history-color) 5%, transparent);
}

.approval-history-toggle:focus-visible {
  outline: 2px solid var(--n-primary-color);
  outline-offset: 2px;
}

.approval-history-chevron {
  flex: 0 0 auto;
  color: var(--n-text-color-3);
}

.approval-history-label {
  flex: 0 0 auto;
  color: var(--approval-history-color);
  font-weight: 600;
  white-space: nowrap;
}

.approval-history-summary {
  flex: 1 1 0;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--n-text-color-2);
}

.approval-history-time {
  flex: 0 0 auto;
  color: var(--n-text-color-3);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.approval-history-body {
  max-height: 240px;
  margin: 4px 0 2px 16px;
  padding: 8px 10px;
  overflow: auto;
  border: 1px solid var(--n-border-color);
  border-radius: 6px;
  background: var(--app-surface-sunken, transparent);
  font-size: 12px;
  line-height: 1.55;
  overflow-wrap: anywhere;
  overscroll-behavior: contain;
}

.approval-history-prompt,
.approval-history-command {
  white-space: pre-wrap;
}

.approval-history-command {
  margin: 0;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
}

.approval-history-prompt + .approval-history-command {
  margin-top: 8px;
}

@media (max-width: 640px) {
  .approval-history-toggle {
    flex-wrap: wrap;
    column-gap: 6px;
    row-gap: 0;
  }

  .approval-history-summary {
    order: 1;
    flex-basis: calc(100% - 16px);
    margin-left: 16px;
  }

  .approval-history-time {
    margin-left: auto;
  }
}
</style>
