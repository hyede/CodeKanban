<script setup lang="ts">
import { ref } from 'vue';

import type { PiActivityGroupRow } from '@/components/web-session/webSessionPiActivityGroup';
import { renderMarkdown } from '@/utils/markdown';

defineProps<{
  rows: PiActivityGroupRow[];
  label: string;
  stepsLabel: string;
  summary?: string;
  time: string;
  timeTitle: string;
  expanded?: boolean;
  streaming?: boolean;
}>();

defineEmits<{ toggle: [] }>();

const openRows = ref<Record<string, boolean>>({});

function isRowOpen(key: string) {
  return Boolean(openRows.value[key]);
}

function toggleRow(key: string) {
  openRows.value = { ...openRows.value, [key]: !openRows.value[key] };
}
</script>

<template>
  <div class="activity-group" :class="{ 'is-streaming': streaming }">
    <button
      type="button"
      class="activity-group-header"
      :aria-expanded="Boolean(expanded)"
      @click="$emit('toggle')"
    >
      <span class="activity-group-caret" aria-hidden="true">{{ expanded ? '▾' : '▸' }}</span>
      <span class="activity-group-label">
        <span v-if="streaming" class="activity-group-dot" aria-hidden="true"></span>
        {{ label }}
      </span>
      <span class="activity-group-steps">{{ stepsLabel }}</span>
      <span v-if="summary" class="activity-group-summary">{{ summary }}</span>
      <span class="activity-group-time" :title="timeTitle">{{ time }}</span>
    </button>

    <div v-if="expanded" class="activity-group-body">
      <template v-for="row in rows" :key="row.key">
        <div v-if="row.note" class="activity-group-note" :title="row.note">{{ row.note }}</div>
        <div v-else class="activity-group-row">
          <button
            type="button"
            class="activity-group-row-header"
            :aria-expanded="isRowOpen(row.key)"
            @click="toggleRow(row.key)"
          >
            <span class="activity-group-caret" aria-hidden="true">{{
              isRowOpen(row.key) ? '▾' : '▸'
            }}</span>
            <span class="activity-group-row-name">{{ row.name }}</span>
            <span v-if="row.summary" class="activity-group-row-summary">{{ row.summary }}</span>
          </button>
          <div
            v-if="isRowOpen(row.key) && row.body"
            class="activity-group-row-body chat-markdown"
            v-html="renderMarkdown(row.body)"
          ></div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.activity-group {
  min-width: 0;
  margin: 2px 0;
  padding-left: 10px;
  border-left: 2px solid var(--n-border-color, rgba(128, 128, 128, 0.22));
}

.activity-group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  max-width: 100%;
  padding: 4px 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.activity-group-caret {
  flex: 0 0 auto;
  opacity: 0.7;
}

.activity-group-label {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 5px;
}

.activity-group-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.75;
  animation: livePulse 1.4s ease-in-out infinite;
}

.activity-group-steps {
  flex: 0 0 auto;
  font-size: 11px;
  opacity: 0.6;
}

.activity-group-summary {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  opacity: 0.6;
}

.activity-group-time {
  flex: 0 0 auto;
  font-size: 11px;
  opacity: 0.65;
}

.activity-group-body {
  padding: 2px 0 4px 6px;
}

.activity-group-row-header {
  display: flex;
  align-items: baseline;
  gap: 6px;
  width: 100%;
  max-width: 100%;
  padding: 2px 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
  opacity: 0.85;
}

.activity-group-row-name {
  flex: 0 0 auto;
  font-weight: 500;
}

.activity-group-row-summary {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  opacity: 0.6;
}

.activity-group-row-body {
  padding: 4px 0 6px 18px;
  font-size: 12px;
}

.activity-group-note {
  padding: 2px 0;
  overflow: hidden;
  font-size: 12px;
  white-space: nowrap;
  text-overflow: ellipsis;
  opacity: 0.55;
}
</style>
