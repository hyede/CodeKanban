// @vitest-environment happy-dom

import { afterEach, describe, expect, it } from 'vitest';

import type { WebSessionBlock } from '@/stores/webSession';
import {
  findWebSessionConversationSearchMatches,
  matchesWebSessionConversationSearchTarget,
  mergeWebSessionConversationSearchMatches,
  resolveWebSessionConversationSearchMatchIndex,
  shouldHandleWebSessionConversationSearchShortcut,
  type WebSessionConversationSearchFilters,
} from '@/components/web-session/webSessionConversationSearch';

const dialogueFilters: WebSessionConversationSearchFilters = {
  user: true,
  assistant: true,
  tools: false,
  system: false,
};

function makeBlock(
  id: string,
  kind: WebSessionBlock['kind'],
  text: string,
  orderIndex: number,
  extra: Partial<WebSessionBlock> = {}
): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex,
    kind,
    itemType: `${kind}_message`,
    text,
    timestamp: orderIndex,
    attachments: [],
    ...extra,
  };
}

describe('webSessionConversationSearch', () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it('locates merged approvals from either the request or response search result', () => {
    const block = makeBlock('response', 'system', 'Approval granted', 3, {
      sourceThreadId: 'thread',
      approvalRequest: { id: 'request', key: 'request-key', timestamp: 1 },
    });
    expect(
      matchesWebSessionConversationSearchTarget(block, {
        id: 'request',
        orderIndex: 1,
        kind: 'system',
        sourceThreadId: 'thread',
      })
    ).toBe(true);
    expect(
      matchesWebSessionConversationSearchTarget(block, {
        id: 'response',
        orderIndex: 3,
        kind: 'system',
        sourceThreadId: 'thread',
      })
    ).toBe(true);
    expect(
      matchesWebSessionConversationSearchTarget(block, {
        id: 'request',
        orderIndex: 1,
        kind: 'system',
        sourceThreadId: 'other-thread',
      })
    ).toBe(false);
  });

  it('handles Ctrl+F and Cmd+F in the regular conversation interface', () => {
    const input = document.createElement('input');
    document.body.append(input);
    input.focus();

    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true })
      )
    ).toBe(true);
    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', metaKey: true })
      )
    ).toBe(true);
  });

  it('leaves Ctrl+F to the browser while a dialog is open', () => {
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    dialog.setAttribute('aria-modal', 'true');
    document.body.append(dialog);

    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true })
      )
    ).toBe(false);
  });

  it('leaves Ctrl+F to the browser when focus is inside a non-modal overlay', () => {
    const menu = document.createElement('div');
    menu.setAttribute('role', 'menu');
    const button = document.createElement('button');
    menu.append(button);
    document.body.append(menu);
    button.focus();

    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true })
      )
    ).toBe(false);
  });

  it('ignores events that are not the unmodified find shortcut', () => {
    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, altKey: true })
      )
    ).toBe(false);
    expect(
      shouldHandleWebSessionConversationSearchShortcut(
        new KeyboardEvent('keydown', { key: 'g', ctrlKey: true })
      )
    ).toBe(false);
  });

  it('searches user and assistant text with the dialogue defaults', () => {
    const matches = findWebSessionConversationSearchMatches(
      [
        makeBlock('user-1', 'user', 'Please inspect the release notes', 1),
        makeBlock('assistant-1', 'assistant', 'I found the release notes', 2),
        makeBlock('tool-1', 'tool', 'release notes tool', 3),
        makeBlock('system-1', 'system', 'release notes approval', 4),
      ],
      'RELEASE',
      dialogueFilters
    );

    expect(matches.map(match => match.id)).toEqual(['user-1', 'assistant-1']);
  });

  it('searches tool input, output, and compacted group items when enabled', () => {
    const block = makeBlock('tool-latest', 'tool', '', 2, {
      itemType: 'command_execution',
      tool: {
        id: 'tool-latest',
        name: 'CommandExecution',
        kind: 'command_execution',
        input: { command: 'git status' },
        output: 'working tree clean',
        status: 'done',
        commandGroup: { id: 'group-1', count: 2 },
      },
      payload: {
        groupItems: [
          {
            toolId: 'tool-old',
            command: 'pnpm test',
            output: 'all tests passed',
          },
        ],
      },
    });
    const matches = findWebSessionConversationSearchMatches([block], 'pnpm test', {
      ...dialogueFilters,
      tools: true,
    });

    expect(matches).toHaveLength(1);
    expect(matches[0]).toMatchObject({
      id: 'tool-latest',
      toolId: 'tool-latest',
      commandGroupId: 'group-1',
    });
  });

  it('counts and orders each occurrence within the same message', () => {
    const matches = findWebSessionConversationSearchMatches(
      [makeBlock('message', 'assistant', 'plan **PLAN** and plan again', 1)],
      'plan',
      dialogueFilters
    );
    expect(matches.map(match => match.occurrenceIndex)).toEqual([0, 1, 2]);
    expect(resolveWebSessionConversationSearchMatchIndex(matches, matches[1], 0)).toBe(1);
  });

  it('counts visible link text without counting its hidden destination', () => {
    const matches = findWebSessionConversationSearchMatches(
      [makeBlock('message', 'assistant', '[plan](https://example.com/plan) and plan', 1)],
      'plan',
      dialogueFilters
    );
    expect(matches).toHaveLength(2);
  });

  it('merges remote history without collapsing repeated local occurrences', () => {
    const local = findWebSessionConversationSearchMatches(
      [makeBlock('local', 'assistant', 'plan plan plan', 2)],
      'plan',
      dialogueFilters
    );
    const merged = mergeWebSessionConversationSearchMatches(
      local,
      [
        { id: 'older', orderIndex: 1, kind: 'user', text: 'plan plan' },
        { id: 'local', orderIndex: 2, kind: 'assistant', text: 'plan plan plan' },
      ],
      'plan'
    );
    expect(merged).toHaveLength(5);
    expect(merged.map(match => match.occurrenceIndex)).toEqual([0, 1, 0, 1, 2]);
    expect(resolveWebSessionConversationSearchMatchIndex(merged, local[1], 0)).toBe(3);
  });

  it('does not merge equal item IDs from different threads', () => {
    const local = findWebSessionConversationSearchMatches(
      [makeBlock('same-id', 'assistant', 'plan plan', 2, { sourceThreadId: 'main' })],
      'plan',
      dialogueFilters
    );
    const remote = {
      id: 'same-id',
      orderIndex: 1,
      kind: 'assistant',
      sourceThreadId: 'child',
      text: 'plan',
    };
    expect(mergeWebSessionConversationSearchMatches(local, [remote], 'plan')).toHaveLength(3);
    expect(
      matchesWebSessionConversationSearchTarget(
        makeBlock('same-id', 'assistant', 'plan', 1, { sourceThreadId: 'main' }),
        remote
      )
    ).toBe(false);
  });

  it('searches system prompts only when system interactions are enabled', () => {
    const block = makeBlock('system-1', 'system', '', 1, {
      itemType: 'approval_request',
      detail: {
        type: 'approval_request',
        prompt: 'Allow the deployment command?',
      },
    });

    expect(findWebSessionConversationSearchMatches([block], 'deployment', dialogueFilters)).toEqual(
      []
    );
    expect(
      findWebSessionConversationSearchMatches([block], 'deployment', {
        ...dialogueFilters,
        system: true,
      })
    ).toHaveLength(1);
  });

  it('deduplicates local and remote matches by item or command group', () => {
    const local = findWebSessionConversationSearchMatches(
      [
        makeBlock('tool-latest', 'tool', '', 2, {
          tool: {
            id: 'tool-latest',
            name: 'CommandExecution',
            status: 'done',
            commandGroup: { id: 'group-1', count: 2 },
          },
        }),
      ],
      'command',
      { ...dialogueFilters, tools: true }
    );
    const merged = mergeWebSessionConversationSearchMatches(local, [
      {
        id: 'tool-old',
        orderIndex: 1,
        kind: 'tool',
        toolId: 'tool-old',
        commandGroupId: 'group-1',
      },
      {
        id: 'tool-latest',
        orderIndex: 2,
        kind: 'tool',
        toolId: 'tool-latest',
        commandGroupId: 'group-1',
      },
    ]);

    expect(merged).toHaveLength(1);
    expect(merged[0].key).toBe('tool-latest');
    expect(
      matchesWebSessionConversationSearchTarget(
        {
          ...makeBlock('tool-latest', 'tool', '', 2),
          tool: {
            id: 'tool-latest',
            name: 'CommandExecution',
            status: 'done',
            commandGroup: { id: 'group-1', count: 2 },
          },
        },
        merged[0]
      )
    ).toBe(true);
  });

  it('maps an ungrouped compacted command item to its visible card', () => {
    const compactedBlock = makeBlock('tool-latest', 'tool', '', 2, {
      tool: {
        id: 'tool-latest',
        name: 'CommandExecution',
        status: 'done',
      },
      payload: {
        groupItems: [{ toolId: 'tool-old', command: 'pnpm test' }],
      },
    });

    expect(
      matchesWebSessionConversationSearchTarget(compactedBlock, {
        id: 'tool-old',
        orderIndex: 1,
        kind: 'tool',
        toolId: 'tool-old',
      })
    ).toBe(true);
  });

  it('keeps the active result when older remote matches are inserted before it', () => {
    const activeMatch = {
      id: 'local-latest',
      orderIndex: 20,
      kind: 'assistant',
    };
    const matches = mergeWebSessionConversationSearchMatches(
      [activeMatch],
      [
        { id: 'remote-oldest', orderIndex: 1, kind: 'user' },
        { id: 'remote-older', orderIndex: 10, kind: 'assistant' },
        activeMatch,
      ]
    );

    expect(resolveWebSessionConversationSearchMatchIndex(matches, activeMatch, 0)).toBe(2);
  });

  it('clamps a missing active result to the requested fallback index', () => {
    const matches = [
      { id: 'first', orderIndex: 1, kind: 'user' },
      { id: 'last', orderIndex: 2, kind: 'assistant' },
    ];

    expect(resolveWebSessionConversationSearchMatchIndex(matches, null, Number.MAX_VALUE)).toBe(1);
  });
});
