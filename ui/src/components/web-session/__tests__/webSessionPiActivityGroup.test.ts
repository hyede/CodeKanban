import { describe, expect, it } from 'vitest';

import type { WebSessionBlock } from '@/stores/webSession';
import {
  buildPiActivityGroupBlock,
  isPiActivityGroupBlock,
  piActivityGroupItems,
  piActivitySummaryText,
  projectPiActivityGroups,
} from '@/components/web-session/webSessionPiActivityGroup';

const TIMESTAMP = Date.UTC(2026, 8, 10, 12, 0, 0);

function toolBlock(
  id: string,
  options: {
    orderIndex?: number;
    name?: string;
    kind?: string;
    status?: 'running' | 'done' | 'error';
    output?: string;
  } = {}
): WebSessionBlock {
  const status = options.status ?? 'done';
  return {
    key: id,
    id,
    orderIndex: options.orderIndex ?? 1,
    kind: 'tool',
    itemType: 'tool',
    text: '',
    timestamp: TIMESTAMP + (options.orderIndex ?? 1) * 1000,
    attachments: [],
    done: status !== 'running',
    tool: {
      id,
      name: options.name ?? 'bash',
      kind: options.kind ?? 'tool',
      status,
      output: options.output ?? '',
    },
  };
}

function noteBlock(id: string, level: 'info' | 'warn' | 'error', text: string): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex: 99,
    kind: 'system',
    itemType: 'note',
    text,
    level,
    timestamp: TIMESTAMP,
    attachments: [],
    done: true,
  };
}

function textBlock(id: string, orderIndex: number): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex,
    kind: 'assistant',
    itemType: 'agent_message',
    text: 'answer',
    timestamp: TIMESTAMP + orderIndex * 1000,
    attachments: [],
    done: true,
  };
}

/** Mirrors the panel rule: tool rows plus informational notes fold. */
function isMember(block: WebSessionBlock): boolean {
  if (block.kind === 'system') {
    return block.itemType === 'note' && block.level === 'info';
  }
  return block.kind === 'tool' && Boolean(block.tool);
}

describe('webSessionPiActivityGroup', () => {
  it('folds adjacent thinking and tool rows into one disclosure', () => {
    const projected = projectPiActivityGroups(
      [
        toolBlock('think', { orderIndex: 1, name: 'Reasoning', kind: 'reasoning', output: 'a\nb' }),
        toolBlock('bash-1', { orderIndex: 2 }),
        toolBlock('bash-2', { orderIndex: 3 }),
        textBlock('answer', 4),
      ],
      isMember
    );

    expect(projected).toHaveLength(2);
    const group = projected[0];
    expect(isPiActivityGroupBlock(group)).toBe(true);
    expect(group.activityGroupItems?.map(item => item.id)).toEqual(['think', 'bash-1', 'bash-2']);
    // The group anchor keeps the first member's position and the last one's run.
    expect(group.key).toBe('pi-activity:think');
    expect(group.orderIndex).toBe(1);
    expect(group.kind).toBe('system');
    expect(group.tool).toBeUndefined();
    expect(projected[1].id).toBe('answer');
  });

  it('leaves a lone tool call as its own card', () => {
    const projected = projectPiActivityGroups(
      [textBlock('answer', 1), toolBlock('bash-1', { orderIndex: 2 })],
      isMember
    );

    expect(projected.map(block => block.id)).toEqual(['answer', 'bash-1']);
    expect(projected.every(block => !isPiActivityGroupBlock(block))).toBe(true);
  });

  it('keeps informational notes inside the fold but lets warnings break it', () => {
    const projected = projectPiActivityGroups(
      [
        toolBlock('bash-1', { orderIndex: 1 }),
        noteBlock('rtk', 'info', 'RTK rewrite: cat a -> rtk read a'),
        toolBlock('bash-2', { orderIndex: 3 }),
        noteBlock('warn', 'warn', 'something failed'),
        toolBlock('bash-3', { orderIndex: 5 }),
      ],
      isMember
    );

    // bash-1 + note + bash-2 fold; the warn note splits and bash-3 stands alone.
    expect(projected.map(block => (isPiActivityGroupBlock(block) ? 'group' : block.id))).toEqual([
      'group',
      'warn',
      'bash-3',
    ]);
    expect(piActivityGroupItems(projected[0]).map(item => item.id)).toEqual([
      'bash-1',
      'rtk',
      'bash-2',
    ]);
  });

  it('marks the group done only when every member settled', () => {
    const running = buildPiActivityGroupBlock([
      toolBlock('a', { status: 'done' }),
      toolBlock('b', { status: 'running' }),
    ]);
    expect(running.done).toBe(false);

    const settled = buildPiActivityGroupBlock([
      toolBlock('a', { status: 'done' }),
      toolBlock('b', { status: 'done' }),
    ]);
    expect(settled.done).toBe(true);
  });

  it('previews the latest thought line, tool output and note text', () => {
    expect(
      piActivitySummaryText(
        toolBlock('think', {
          name: 'Reasoning',
          kind: 'reasoning',
          output: '# Plan\n\n**Refining**',
        })
      )
    ).toBe('Refining');
    expect(piActivitySummaryText(toolBlock('bash', { output: 'npm  warn   x' }))).toBe(
      'npm warn x'
    );
    expect(piActivitySummaryText(toolBlock('bash', { output: '' }))).toBe('bash');
    expect(piActivitySummaryText(noteBlock('n', 'info', 'RTK  rewrite: a -> b'))).toBe(
      'RTK rewrite: a -> b'
    );
  });
});
