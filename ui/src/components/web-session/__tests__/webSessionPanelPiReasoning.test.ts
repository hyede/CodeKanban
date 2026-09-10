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
});

describe('WebSessionPanel Pi activity folding', () => {
  it('folds Pi activity runs after the shared visible projection', () => {
    expect(panelSource).toMatch(/projectPiActivityGroups\(projected, isPiActivityGroupMember\)/);
    expect(panelSource).toMatch(
      /currentSession\.value\?\.agent !== 'pi'[\s\S]{0,80}return projected;/
    );
  });

  it('keeps plan cards, prompts, sub-agent rows and real warnings out of the fold', () => {
    expect(panelSource).toMatch(/function isPiActivityGroupMember\(block: WebSessionBlock\)/);
    expect(panelSource).toMatch(/if \(timelineSubAgent\(block\)\) \{\s*return false;/);
    expect(panelSource).toMatch(
      /if \(isPlanTool\(block\.tool\) \|\| isInteractiveDynamicTool\(block\.tool\)\) \{\s*return false;/
    );
    expect(panelSource).toMatch(/block\.itemType === 'note' && block\.level === 'info'/);
  });

  it('auto-opens a live Pi run and folds it once every step settles', () => {
    expect(panelSource).toMatch(/function isActivityGroupStreaming\(group: WebSessionBlock\)/);
    expect(panelSource).toMatch(/item\.tool\?\.status === 'running'/);
    expect(panelSource).toMatch(/function isActivityGroupExpanded\(group: WebSessionBlock\)/);
    expect(panelSource).toMatch(/return isActivityGroupStreaming\(group\);/);
    expect(panelSource).toMatch(/function toggleActivityGroup\(group: WebSessionBlock\)/);
  });

  it('renders the fold through the low-key activity group row', () => {
    expect(panelSource).toMatch(/v-else-if="isPiActivityGroupBlock\(item\)"/);
    expect(panelSource).toMatch(/:rows="activityGroupRows\(item\)"/);
    expect(panelSource).toMatch(
      /if \(isReasoningDisclosureBlock\(item\) \|\| isPiActivityGroupBlock\(item\)\)/
    );
  });
});
