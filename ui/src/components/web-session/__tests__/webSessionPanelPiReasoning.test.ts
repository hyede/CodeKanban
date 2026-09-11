import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

// Contract scan: the Pi reasoning exception lives inside the panel's script
// setup, so it is asserted against the source instead of a mounted panel.
const panelSource = readFileSync(new URL('../WebSessionPanel.vue', import.meta.url), 'utf8');

describe('WebSessionPanel Pi reasoning disclosure', () => {
  it('surfaces Pi thinking even when the global reasoning switch is off', () => {
    expect(panelSource).toMatch(
      /!showWebSessionReasoning\.value && isReasoningBlock\(block\) && !isPiReasoningBlock\(block\)/
    );
    expect(panelSource).toMatch(/function isPiReasoningBlock\(block: WebSessionBlock\)/);
    expect(panelSource).toMatch(
      /currentSession\.value\?\.agent === 'pi' && isReasoningBlock\(block\)/
    );
  });

  it('auto-opens live Pi thinking and folds it once the segment settles', () => {
    expect(panelSource).toMatch(
      /function isReasoningDisclosureExpanded\(tool: NonNullable<WebSessionBlock\['tool'\]>\)/
    );
    expect(panelSource).toMatch(
      /currentSession\.value\?\.agent === 'pi' && tool\.status === 'running'/
    );
    expect(panelSource).toMatch(
      /function toggleReasoningDisclosure\(tool: NonNullable<WebSessionBlock\['tool'\]>\)/
    );
    // A user click claims the disclosure: the manual value must win over the
    // automatic "running" default.
    expect(panelSource).toMatch(
      /const claimed = expandedTools\.value\[tool\.id\];[\s\S]*?if \(claimed !== undefined\) \{\s*return claimed;/
    );
  });

  it('renders Pi and Codex thinking through the shared low-key disclosure header', () => {
    expect(panelSource).toMatch(/function isReasoningDisclosureBlock\(block: WebSessionBlock\)/);
    expect(panelSource).toMatch(/v-else-if="isReasoningDisclosureBlock\(item\) && item\.tool"/);
    expect(panelSource).toMatch(/:summary="reasoningDisclosurePreview\(item\)"/);
    expect(panelSource).toMatch(/:streaming="isReasoningStreaming\(item\)"/);
    // Reasoning rows stay text-only: no tool card chrome is introduced.
    expect(panelSource).toMatch(/function reasoningDisclosurePreview\(block: WebSessionBlock\)/);
  });

  it('folds Pi tool rows through the shared compact projection, by adjacency', () => {
    expect(panelSource).toMatch(
      /projectWebSessionVisibleTimelineBlocks\(\s*filteredTimelineBlocks\.value,\s*currentSession\.value\?\.agent\s*\)/
    );
    expect(panelSource).not.toMatch(/projectPiActivityGroups/);
    expect(panelSource).not.toMatch(/isPiActivityGroupMember/);
  });
});

describe('WebSessionPanel streaming render cost', () => {
  it('routes Pi thinking through the throttled streaming markdown controller', () => {
    // The streaming surfaces are the only ones allowed to bypass a full
    // re-render per delta; thinking has to be one of them.
    expect(panelSource).toMatch(/type StreamingMarkdownSurface = 'message' \| 'plan' \| 'reasoning'/);
    expect(panelSource).toMatch(/function isStreamingReasoningMarkdownBlock\(block: WebSessionBlock\)/);
    expect(panelSource).toMatch(/key: buildStreamingMarkdownKey\(block, 'reasoning'\)/);
    expect(panelSource).toMatch(/getEffectiveStreamingMarkdownText\(block, 'reasoning'\)/);
  });

  it('streams thinking through the block-by-block markdown renderer', () => {
    // Settled blocks keep their HTML across deltas, so streaming bodies render
    // markdown without re-parsing the whole body on every chunk.
    expect(panelSource).toMatch(
      /import WebSessionStreamingMarkdown from '@\/components\/web-session\/WebSessionStreamingMarkdown\.vue';/
    );
    expect(panelSource).toMatch(/:blocks="getReasoningStreamingBlocks\(item\)"/);
    expect(panelSource).toMatch(/:blocks="getMessageStreamingBlocks\(item\)"/);
    expect(panelSource).toMatch(/renderStreamingMarkdownBlocks\(`reasoning:\$\{block\.key\}`/);
    expect(panelSource).toMatch(/:blocks="getReasoningStreamingBlocks\(item\)"\s*\/>/);
    // The settled branch still renders the cached whole-body markdown.
    expect(panelSource).toMatch(/v-html="renderMarkdown\(getReasoningMarkdownText\(item\)\)"/);
    // Neither branch may read the raw output directly any more.
    expect(panelSource).not.toMatch(/renderMarkdown\(item\.tool\.output \|\| ''\)/);
  });

  it('streams plan cards through the same incremental renderer', () => {
    expect(panelSource).toMatch(/v-else-if="isStreamingPlanMarkdownBlock\(item\)"/);
    expect(panelSource).toMatch(/:blocks="getPlanStreamingBlocks\(item\)"/);
    expect(panelSource).toContain('`plan:${block.key}`');
  });

  it('keeps the reasoning preview off a full-body split', () => {
    const previewStart = panelSource.indexOf('function reasoningDisclosurePreview');
    expect(previewStart).toBeGreaterThan(-1);
    const previewBody = panelSource.slice(previewStart, previewStart + 700);

    expect(previewBody).toContain('lastIndexOf(');
    expect(previewBody).not.toContain('.split(');
  });

  it('coalesces pending auto-scroll runs instead of cancelling them', () => {
    expect(panelSource).toMatch(/let timelineScrollSyncScheduled = false;/);
    expect(panelSource).toMatch(/if \(timelineScrollSyncScheduled && !force\) \{\s*return;/);
    expect(panelSource).toMatch(
      /function invalidateTimelineScrollSync\(\) \{\s*timelineScrollSyncVersion \+= 1;\s*timelineScrollSyncScheduled = false;/
    );
  });

  it('strips Magic Context markers from tool output rows', () => {
    expect(panelSource).toMatch(/stripMagicContextTags\(item\.tool\.output\)/);
  });
});
