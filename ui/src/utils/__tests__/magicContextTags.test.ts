import { describe, expect, it, beforeEach } from 'vitest';

import {
  resetMagicContextTagCache,
  stripMagicContextTags,
} from '@/utils/magicContextTags';

describe('stripMagicContextTags', () => {
  beforeEach(() => {
    resetMagicContextTagCache();
  });

  it('returns the value untouched when there are no markers', () => {
    const value = 'plain reasoning without markers';
    expect(stripMagicContextTags(value)).toBe(value);
    expect(stripMagicContextTags('')).toBe('');
  });

  it('removes a marker list together with its separators', () => {
    // This is the shape Magic Context tool results take, which used to render
    // as a bare list of §N§ markers in the timeline.
    expect(
      stripMagicContextTags('Queued: deferred drop §368§, §369§, §370§, §377§.')
    ).toBe('Queued: deferred drop.');
  });

  it('removes an inline reference without leaving a double space', () => {
    expect(stripMagicContextTags('§168§ shows wireHistoryItem is not there')).toBe(
      'shows wireHistoryItem is not there'
    );
    expect(stripMagicContextTags('check §29§ and §30§ output')).toBe('check and output');
  });

  it('keeps unrelated section signs and text intact', () => {
    expect(stripMagicContextTags('see § 5 of the spec')).toBe('see § 5 of the spec');
    expect(stripMagicContextTags('cost is 5§ not a marker')).toBe('cost is 5§ not a marker');
  });

  it('serves repeat lookups from the cache', () => {
    const first = stripMagicContextTags('§1§ cached value');
    const second = stripMagicContextTags('§1§ cached value');
    expect(second).toBe(first);
    resetMagicContextTagCache();
    expect(stripMagicContextTags('§1§ cached value')).toBe(first);
  });
});
