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
