import type { WebSessionBlock } from '@/stores/webSession';

/**
 * Synthetic item type marking a folded Pi activity run.
 *
 * Pi reports every tool call with the same generic kind, so the shared compact
 * tool grouping never fires for it. Instead of classifying tool names (a list
 * that dynamic MCP/extension tools can never complete) the desktop client folds
 * *adjacent* rows, and so do we: consecutive thinking rows, tool calls and
 * informational notes become one disclosure.
 */
export const PI_ACTIVITY_GROUP_ITEM_TYPE = 'pi_activity_group';

export interface PiActivityGroupRow {
  key: string;
  name: string;
  /** One-line preview; empty for informational notes. */
  summary: string;
  /** Rendered inline instead of a collapsible row. */
  note: string;
  /** Full body revealed when the row is expanded. */
  body: string;
}

export function isPiActivityGroupBlock(block: WebSessionBlock): boolean {
  return block.itemType === PI_ACTIVITY_GROUP_ITEM_TYPE;
}

export function piActivityGroupItems(block: WebSessionBlock): WebSessionBlock[] {
  return Array.isArray(block.activityGroupItems) ? block.activityGroupItems : [];
}

export function buildPiActivityGroupBlock(members: WebSessionBlock[]): WebSessionBlock {
  const first = members[0];
  const last = members[members.length - 1];
  return {
    key: `pi-activity:${first.key}`,
    id: `pi-activity:${first.id}`,
    sourceThreadId: null,
    sourceTurnId: last.sourceTurnId ?? null,
    runId: last.runId ?? null,
    runDurationMs: last.runDurationMs ?? null,
    runOutcome: last.runOutcome ?? null,
    orderIndex: first.orderIndex,
    eventSequence: last.eventSequence,
    kind: 'system',
    itemType: PI_ACTIVITY_GROUP_ITEM_TYPE,
    text: '',
    timestamp: first.timestamp,
    observedAt: last.observedAt ?? null,
    attachments: [],
    payload: {},
    done: members.every(member => member.done === true),
    activityGroupItems: members,
  };
}

/**
 * Fold runs of adjacent Pi activity rows into one disclosure.
 *
 * Runs shorter than `minMembers` are left untouched, so a lone tool call keeps
 * its full card (copy/raw toggles, detail dialog) instead of gaining a wrapper.
 */
export function projectPiActivityGroups(
  blocks: WebSessionBlock[],
  isMember: (block: WebSessionBlock) => boolean,
  minMembers = 2
): WebSessionBlock[] {
  const projected: WebSessionBlock[] = [];
  let run: WebSessionBlock[] = [];

  const flush = () => {
    if (run.length === 0) {
      return;
    }
    if (run.length >= minMembers) {
      projected.push(buildPiActivityGroupBlock(run));
    } else {
      projected.push(...run);
    }
    run = [];
  };

  for (const block of blocks) {
    if (isMember(block)) {
      run.push(block);
      continue;
    }
    flush();
    projected.push(block);
  }
  flush();

  return projected;
}

function toolKindOf(block: WebSessionBlock): string {
  return String(block.tool?.kind ?? block.tool?.meta?.kind ?? '').trim();
}

function lastMeaningfulLine(value: string): string {
  const lines = value
    .split('\n')
    .map(line => line.replace(/^#+\s*|\*\*/g, '').trim())
    .filter(Boolean);
  return lines[lines.length - 1] ?? '';
}

/** One-line preview of a folded row, used by the group header and its rows. */
export function piActivitySummaryText(block: WebSessionBlock): string {
  if (block.kind === 'system') {
    return block.text.replace(/\s+/g, ' ').trim();
  }
  const tool = block.tool;
  if (!tool) {
    return block.text.replace(/\s+/g, ' ').trim();
  }
  if (toolKindOf(block) === 'reasoning') {
    return lastMeaningfulLine(String(tool.output ?? ''));
  }
  const output = String(tool.output ?? '')
    .replace(/\s+/g, ' ')
    .trim();
  if (output) {
    return output.slice(0, 160);
  }
  return String(tool.name ?? '').trim();
}
