/**
 * Magic Context (a Pi extension) tags every block it feeds the model with a
 * `§N§` marker so the model can reference or drop it. Models then quote those
 * markers in their own thinking, and Pi persists that thinking verbatim — so
 * the raw markers show up in transcripts even though they only ever meant
 * something to the model. They are pure bookkeeping for humans, so the display
 * layer drops them.
 */
const MAGIC_CONTEXT_TAG_PATTERN = /§\d+§\s*[,，、;；]?\s*/g;

const STRIPPED_TAG_CACHE = new Map<string, string>();
const STRIPPED_TAG_CACHE_LIMIT = 400;

/**
 * Remove Magic Context `§N§` markers, along with the separator that glued them
 * into a list, so `"drop §368§, §369§."` reads as `"drop."` rather than
 * `"drop , ."`. Cached because it runs on every timeline render for texts that
 * rarely change.
 */
export function stripMagicContextTags(value: string): string {
  if (!value || !value.includes('§')) {
    return value;
  }

  const cached = STRIPPED_TAG_CACHE.get(value);
  if (cached !== undefined) {
    STRIPPED_TAG_CACHE.delete(value);
    STRIPPED_TAG_CACHE.set(value, cached);
    return cached;
  }

  const stripped = value
    .replace(MAGIC_CONTEXT_TAG_PATTERN, '')
    .replace(/ {1,}([.,;:])/g, '$1')
    .replace(/[ \t]{2,}/g, ' ');

  STRIPPED_TAG_CACHE.set(value, stripped);
  if (STRIPPED_TAG_CACHE.size > STRIPPED_TAG_CACHE_LIMIT) {
    const oldest = STRIPPED_TAG_CACHE.keys().next().value;
    if (oldest !== undefined) {
      STRIPPED_TAG_CACHE.delete(oldest);
    }
  }
  return stripped;
}

/** Test seam: the cache must not leak between cases. */
export function resetMagicContextTagCache() {
  STRIPPED_TAG_CACHE.clear();
}
