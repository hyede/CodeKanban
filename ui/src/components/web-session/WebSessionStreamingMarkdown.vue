<script setup lang="ts">
import type { StreamingMarkdownBlock } from '@/utils/markdown';

defineProps<{ blocks: StreamingMarkdownBlock[] }>();
</script>

<template>
  <div class="streaming-markdown">
    <!--
      Each block keeps a stable key and is memoized on its HTML: settled blocks
      reuse the identical string, so Vue skips patching them while the tail
      block streams.
    -->
    <div
      v-for="block in blocks"
      :key="block.key"
      v-memo="[block.html]"
      class="streaming-markdown-block"
      v-html="block.html"
    ></div>
  </div>
</template>

<style scoped>
/* `display: contents` keeps block wrappers out of the layout so the existing
   descendant styles in chat-markdown.css apply unchanged. */
.streaming-markdown-block {
  display: contents;
}

/* The direct-child rules in chat-markdown.css now match the wrappers, so the
   first/last spacing has to be re-applied to their contents. */
.streaming-markdown > .streaming-markdown-block:first-child > :first-child {
  margin-top: 0;
}

.streaming-markdown > .streaming-markdown-block:last-child > :last-child {
  margin-bottom: 0;
}
</style>
