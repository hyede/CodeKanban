import { describe, expect, it, vi } from 'vitest';

import type { WebSessionCommandExecutionGroupDetail } from '@/api/webSession';
import type { WebSessionBlock } from '@/stores/webSession';
import {
  isEmptyAssistantBlock,
  isTransportRetryNoteBlock,
  loadWebSessionCompactToolDetail,
  projectWebSessionCompactTimelineBlocks,
  projectWebSessionVisibleTimelineBlocks,
} from '@/components/web-session/webSessionCompactTimeline';

function buildFileChangeBlock(
  id: string,
  options: {
    key?: string;
    orderIndex?: number;
    path?: string;
    groupId?: string;
    count?: number;
    groupItems?: Array<Record<string, unknown>>;
    status?: 'running' | 'done' | 'error';
    timestamp?: number;
    sourceThreadId?: string;
  } = {}
): WebSessionBlock {
  const timestamp = options.timestamp ?? Date.UTC(2026, 3, 20, 12, 0, 0);
  return {
    key: options.key ?? id,
    id,
    orderIndex: options.orderIndex ?? 1,
    kind: 'tool',
    itemType: 'file_change',
    text: '',
    timestamp,
    sourceThreadId: options.sourceThreadId,
    attachments: [],
    tool: {
      id,
      name: 'FileChange',
      kind: 'file_change',
      input: {
        path: options.path ?? `src/${id}.ts`,
        changes: [{ path: options.path ?? `src/${id}.ts` }],
      },
      status: options.status ?? 'done',
      meta: {
        kind: 'file_change',
        title: 'FileChange',
        subtitle: options.path ?? `src/${id}.ts`,
      },
      ...(options.groupId
        ? {
            commandGroup: {
              id: options.groupId,
              count: options.count ?? 1,
            },
          }
        : {}),
    },
    ...(options.groupItems
      ? {
          payload: {
            groupItems: options.groupItems,
          },
        }
      : {}),
  };
}

function buildCommandBlock(
  id: string,
  options: {
    orderIndex?: number;
    command?: string;
    groupId?: string;
    count?: number;
    status?: 'running' | 'done' | 'error';
    timestamp?: number;
    sourceThreadId?: string;
  } = {}
): WebSessionBlock {
  const command = options.command ?? id;
  const timestamp = options.timestamp ?? Date.UTC(2026, 3, 20, 12, 0, 0);
  return {
    key: id,
    id,
    orderIndex: options.orderIndex ?? 1,
    kind: 'tool',
    itemType: 'command_execution',
    text: '',
    timestamp,
    sourceThreadId: options.sourceThreadId,
    attachments: [],
    tool: {
      id,
      name: 'CommandExecution',
      kind: 'command_execution',
      input: { command },
      output: `${command} output`,
      status: options.status ?? 'done',
      meta: {
        kind: 'command_execution',
        title: 'CommandExecution',
        subtitle: command,
      },
      ...(options.groupId
        ? {
            commandGroup: {
              id: options.groupId,
              count: options.count ?? 1,
            },
          }
        : {}),
    },
  };
}

function buildDynamicToolBlock(
  id: string,
  name: string,
  input: Record<string, unknown>,
  options: {
    orderIndex?: number;
    groupId?: string;
    count?: number;
    status?: 'running' | 'done' | 'error';
    timestamp?: number;
    sourceThreadId?: string;
  } = {}
): WebSessionBlock {
  const timestamp = options.timestamp ?? Date.UTC(2026, 3, 20, 12, 0, 0);
  return {
    key: id,
    id,
    orderIndex: options.orderIndex ?? 1,
    kind: 'tool',
    itemType: 'dynamic_tool_call',
    text: '',
    timestamp,
    sourceThreadId: options.sourceThreadId,
    attachments: [],
    tool: {
      id,
      name,
      kind: 'dynamic_tool_call',
      input,
      output: `${name} output`,
      status: options.status ?? 'done',
      meta: {
        kind: 'dynamic_tool_call',
        title: name,
      },
      ...(options.groupId
        ? {
            commandGroup: {
              id: options.groupId,
              count: options.count ?? 1,
            },
          }
        : {}),
    },
  };
}

function buildMessageBlock(id: string, orderIndex: number): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex,
    kind: 'assistant',
    itemType: 'agent_message',
    text: `message-${id}`,
    timestamp: Date.UTC(2026, 3, 20, 12, 0, orderIndex),
    attachments: [],
  };
}

function buildApprovalBlock(
  id: string,
  type: 'approval_request' | 'approval_response',
  orderIndex: number,
  extra: Partial<WebSessionBlock> = {}
): WebSessionBlock {
  return {
    key: id,
    id,
    orderIndex,
    kind: 'system',
    itemType: type,
    text:
      type === 'approval_request'
        ? 'Calling browser_run_code_unsafe from playwright'
        : 'Approval granted',
    timestamp: Date.UTC(2026, 9, 2, 5, 20, 20) + orderIndex * 1000,
    attachments: [],
    detail: {
      type,
      prompt: 'Calling browser_run_code_unsafe from playwright',
      ...(type === 'approval_response' ? { action: 'approve' } : {}),
    },
    ...extra,
  };
}

function readGroupItems(block: WebSessionBlock): Array<Record<string, unknown>> {
  const raw = block.payload?.groupItems;
  return Array.isArray(raw) ? (raw as Array<Record<string, unknown>>) : [];
}

function buildToolDetail(
  groupId: string,
  toolIds: string[],
  status: 'running' | 'done' | 'error' = 'done'
): WebSessionCommandExecutionGroupDetail {
  return {
    groupId,
    kind: 'command_execution',
    title: 'CommandExecution',
    summary: groupId,
    count: toolIds.length,
    firstSeq: 1,
    lastSeq: toolIds.length * 2,
    status,
    latestToolId: toolIds[toolIds.length - 1],
    items: toolIds.map(toolId => ({
      toolId,
      kind: 'command_execution',
      title: 'CommandExecution',
      summary: toolId,
      command: toolId,
      input: { command: toolId },
      output: ('full output for ' + toolId).repeat(300),
      status,
      timestamp: '2026-04-20T12:00:00.000Z',
    })),
  };
}

describe('compact tool detail loading', () => {
  it('loads Devin synthetic command groups through their original tool IDs', async () => {
    const blocks = [buildCommandBlock('call-1'), buildCommandBlock('call-2')];
    const [projected] = projectWebSessionCompactTimelineBlocks(blocks, 'devin');
    const load = vi.fn(async (id: string) => {
      if (!['call-1', 'call-2'].includes(id)) {
        throw new Error('tool group not found');
      }
      return buildToolDetail(id, [id]);
    });

    const detail = await loadWebSessionCompactToolDetail(projected!, load);

    expect(load.mock.calls).toEqual([['call-1'], ['call-2']]);
    expect(detail.groupId).toBe(projected!.tool!.commandGroup!.id);
    expect(detail.count).toBe(2);
    expect(detail.items.map(item => item.toolId)).toEqual(['call-1', 'call-2']);
    expect(detail.items[0]!.output).toBe(buildToolDetail('call-1', ['call-1']).items[0]!.output);
  });

  it('loads every persisted group when adjacent Devin rows have different group IDs', async () => {
    const [projected] = projectWebSessionCompactTimelineBlocks(
      [
        buildCommandBlock('call-2', { groupId: 'group-1', count: 2 }),
        buildCommandBlock('call-4', { groupId: 'group-2', count: 2 }),
      ],
      'devin'
    );
    const first = buildToolDetail('group-1', ['call-1', 'call-2']);
    const second = buildToolDetail('group-2', ['call-3', 'call-4']);
    second.firstSeq = 5;
    second.lastSeq = 8;
    const load = vi.fn(async (id: string) => (id === 'group-1' ? first : second));

    const detail = await loadWebSessionCompactToolDetail(projected!, load);

    expect(load.mock.calls).toEqual([['group-1'], ['group-2']]);
    expect(detail.items.map(item => item.toolId)).toEqual(['call-1', 'call-2', 'call-3', 'call-4']);
    expect(detail).toMatchObject({ count: 4, firstSeq: 1, lastSeq: 8, latestToolId: 'call-4' });
  });

  it('requests shared persisted groups once and keeps all server details', async () => {
    const [projected] = projectWebSessionCompactTimelineBlocks([
      buildCommandBlock('call-1', { groupId: 'group-1' }),
      buildCommandBlock('call-2', { groupId: 'group-1', count: 3 }),
    ]);
    const fullDetail = buildToolDetail('group-1', ['call-0', 'call-1', 'call-2']);
    const load = vi.fn(async () => fullDetail);

    expect(await loadWebSessionCompactToolDetail(projected!, load)).toBe(fullDetail);
    expect(load.mock.calls).toEqual([['group-1']]);
  });

  it('preserves original source IDs when synthetic file-change rows are folded again', async () => {
    const [first] = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('edit-1'),
      buildFileChangeBlock('edit-2'),
    ]);
    const [projected] = projectWebSessionCompactTimelineBlocks([
      first!,
      buildFileChangeBlock('edit-3'),
    ]);
    const load = vi.fn(async (id: string) => buildToolDetail(id, [id]));

    const detail = await loadWebSessionCompactToolDetail(projected!, load);

    expect(load.mock.calls).toEqual([['edit-1'], ['edit-2'], ['edit-3']]);
    expect(detail.count).toBe(3);
  });

  it('deduplicates overlapping group items while preserving current tool status', async () => {
    const [projected] = projectWebSessionCompactTimelineBlocks([
      buildCommandBlock('call-1', { groupId: 'group-1' }),
      buildCommandBlock('call-2', { groupId: 'group-2' }),
    ]);
    const first = buildToolDetail('group-1', ['call-1', 'call-2']);
    const second = buildToolDetail('group-2', ['call-2'], 'running');
    const load = vi.fn(async (id: string) => (id === 'group-1' ? first : second));

    const detail = await loadWebSessionCompactToolDetail(projected!, load);

    expect(detail.items.map(item => item.toolId)).toEqual(['call-1', 'call-2']);
    expect(detail.items[1]!.status).toBe('running');
    expect(detail.status).toBe('running');
    expect(detail.count).toBe(2);
  });

  it('loads single tools by their stored ID', async () => {
    const block = buildDynamicToolBlock('read-1', 'Read file', { path: 'src/main.ts' });
    const fullDetail = buildToolDetail('read-1', ['read-1']);
    const load = vi.fn(async () => fullDetail);

    expect(await loadWebSessionCompactToolDetail(block, load)).toBe(fullDetail);
    expect(load.mock.calls).toEqual([['read-1']]);
  });

  it('reports source failures instead of presenting partial group details as complete', async () => {
    const [projected] = projectWebSessionCompactTimelineBlocks([
      buildCommandBlock('call-1'),
      buildCommandBlock('call-2'),
    ]);
    const load = vi.fn(async (id: string) => {
      if (id === 'call-2') {
        throw new Error('failed to load tool group');
      }
      return buildToolDetail(id, [id]);
    });

    await expect(loadWebSessionCompactToolDetail(projected!, load)).rejects.toThrow(
      'failed to load tool group'
    );
  });
});

describe('approval history projection', () => {
  it.each(['approve', 'reject', 'cancel'])(
    'merges a request and its %s response without changing stored blocks',
    action => {
      const request = buildApprovalBlock('request', 'approval_request', 1, {
        sourceItemId: 'tool-1',
        detail: {
          type: 'approval_request',
          prompt: 'Run tool',
          command: 'browser command',
          approvalKind: 'mcp',
        },
      });
      const response = buildApprovalBlock('response', 'approval_response', 3, {
        payload: { iid: 'tool-1' },
        detail: { type: 'approval_response', action },
      });
      const message = buildMessageBlock('between', 2);
      const original = structuredClone([request, message, response]);

      const visible = projectWebSessionVisibleTimelineBlocks([request, message, response], 'devin');

      expect(visible).toHaveLength(2);
      expect(visible[0]).toBe(message);
      expect(visible[1]).toMatchObject({
        id: response.id,
        key: response.key,
        orderIndex: response.orderIndex,
        timestamp: response.timestamp,
        detail: {
          type: 'approval_response',
          prompt: 'Run tool',
          command: 'browser command',
          approvalKind: 'mcp',
          action,
        },
        approvalRequest: { id: request.id, key: request.key, timestamp: request.timestamp },
      });
      expect([request, message, response]).toEqual(original);
    }
  );

  it('matches legacy responses without IDs by prompt and inherits the request command', () => {
    const request = buildApprovalBlock('request', 'approval_request', 1);
    request.detail!.command = 'browser command';
    const response = buildApprovalBlock('response', 'approval_response', 2);

    const visible = projectWebSessionVisibleTimelineBlocks([request, response]);

    expect(visible).toHaveLength(1);
    expect(visible[0]!.detail!.command).toBe('browser command');
  });

  it('keeps repeated approvals for the same tool as separate interactions', () => {
    const first = buildApprovalBlock('request-1', 'approval_request', 1);
    const second = buildApprovalBlock('request-2', 'approval_request', 3);
    const visible = projectWebSessionVisibleTimelineBlocks([
      first,
      buildApprovalBlock('response-1', 'approval_response', 2),
      second,
      buildApprovalBlock('response-2', 'approval_response', 4),
    ]);

    expect(visible.map(block => block.id)).toEqual(['response-1', 'response-2']);
    expect(visible.map(block => block.approvalRequest?.id)).toEqual(['request-1', 'request-2']);
  });

  it('uses request IDs rather than identical prompts to match concurrent approvals', () => {
    const visible = projectWebSessionVisibleTimelineBlocks([
      buildApprovalBlock('request-1', 'approval_request', 1, { sourceItemId: 'tool-1' }),
      buildApprovalBlock('request-2', 'approval_request', 2, { sourceItemId: 'tool-2' }),
      buildApprovalBlock('response-1', 'approval_response', 3, { payload: { iid: 'tool-1' } }),
    ]);

    expect(visible.map(block => block.id)).toEqual(['request-2', 'response-1']);
    expect(visible[1]!.approvalRequest?.id).toBe('request-1');
  });

  it.each([
    { sourceThreadId: 'other-thread' },
    { sourceTurnId: 'other-turn' },
    { runId: 'other-run' },
    { payload: { iid: 'other-tool' } },
    { timestamp: 1 },
    {
      detail: {
        type: 'approval_response' as const,
        prompt: 'Different operation',
        action: 'approve',
      },
    },
    {
      detail: {
        type: 'approval_response' as const,
        prompt: 'Run tool',
        command: 'different command',
        action: 'approve',
      },
    },
  ])('does not merge unrelated approvals: %j', extra => {
    const request = buildApprovalBlock('request', 'approval_request', 1, {
      sourceThreadId: 'thread',
      sourceTurnId: 'turn',
      runId: 'run',
      sourceItemId: 'tool',
      detail: { type: 'approval_request', prompt: 'Run tool', command: 'command' },
    });
    const response = buildApprovalBlock('response', 'approval_response', 2, {
      sourceThreadId: 'thread',
      sourceTurnId: 'turn',
      runId: 'run',
      detail: {
        type: 'approval_response',
        prompt: 'Run tool',
        command: 'command',
        action: 'approve',
      },
      ...extra,
    });

    expect(projectWebSessionVisibleTimelineBlocks([request, response])).toEqual([
      request,
      response,
    ]);
  });

  it('keeps unmatched requests and responses and never matches across a user message', () => {
    const request = buildApprovalBlock('request', 'approval_request', 1);
    const user = { ...buildMessageBlock('user', 2), kind: 'user' as const };
    const response = buildApprovalBlock('response', 'approval_response', 3);

    expect(projectWebSessionVisibleTimelineBlocks([request])).toEqual([request]);
    expect(projectWebSessionVisibleTimelineBlocks([response])).toEqual([response]);
    expect(projectWebSessionVisibleTimelineBlocks([request, user, response])).toEqual([
      request,
      user,
      response,
    ]);
  });

  it('preserves approval boundaries between compact tool groups', () => {
    const visible = projectWebSessionVisibleTimelineBlocks([
      buildCommandBlock('before', { orderIndex: 1 }),
      buildApprovalBlock('request', 'approval_request', 2),
      buildCommandBlock('during', { orderIndex: 3 }),
      buildApprovalBlock('response', 'approval_response', 4),
      buildCommandBlock('after', { orderIndex: 5 }),
    ]);

    expect(visible.map(block => block.id)).toEqual(['before', 'during', 'response', 'after']);
    expect(visible.every(block => !block.tool?.commandGroup)).toBe(true);
  });
});

describe('webSessionCompactTimeline', () => {
  it('identifies transport retry notes without treating ordinary notes as retries', () => {
    const retryNote: WebSessionBlock = {
      key: 'retry-note',
      id: 'retry-note',
      orderIndex: 1,
      kind: 'system',
      itemType: 'note',
      text: 'Claude API retry 2/10 after 1s (server_error)',
      timestamp: Date.UTC(2026, 3, 20, 12, 0, 0),
      attachments: [],
      payload: { code: 'transport_retrying' },
    };
    const ordinaryNote = { ...retryNote, key: 'note', id: 'note', payload: { lvl: 'warn' } };

    expect(isTransportRetryNoteBlock(retryNote)).toBe(true);
    expect(isTransportRetryNoteBlock(ordinaryNote)).toBe(false);
  });

  it('hides retry notes after preserving their boundaries between tool groups', () => {
    const retryNote: WebSessionBlock = {
      key: 'retry-note',
      id: 'retry-note',
      orderIndex: 2,
      kind: 'system',
      itemType: 'note',
      text: 'Claude API retry 2/10 after 1s (server_error)',
      timestamp: Date.UTC(2026, 3, 20, 12, 0, 1),
      attachments: [],
      payload: { code: 'transport_retrying' },
    };
    const visible = projectWebSessionVisibleTimelineBlocks([
      buildCommandBlock('before-retry', { orderIndex: 1, command: 'git status' }),
      retryNote,
      buildCommandBlock('after-retry', { orderIndex: 3, command: 'git diff' }),
    ]);

    expect(visible).toHaveLength(2);
    expect(visible.map(block => block.tool?.id)).toEqual(['before-retry', 'after-retry']);
    expect(visible.every(block => !block.tool?.commandGroup)).toBe(true);
  });

  it('hides finished assistant placeholders that never produced content', () => {
    const thinkingOnly: WebSessionBlock = {
      key: 'assistant-thinking-only',
      id: 'assistant-thinking-only',
      orderIndex: 1,
      kind: 'assistant',
      itemType: 'agent_message',
      text: '',
      timestamp: Date.UTC(2026, 3, 20, 12, 0, 0),
      attachments: [],
      done: true,
    };

    expect(isEmptyAssistantBlock(thinkingOnly)).toBe(true);
    expect(isEmptyAssistantBlock({ ...thinkingOnly, text: 'answer' })).toBe(false);
    expect(isEmptyAssistantBlock({ ...thinkingOnly, done: false })).toBe(false);
    expect(
      isEmptyAssistantBlock({
        ...thinkingOnly,
        attachments: [{ id: 'a1', name: 'a.png' }],
      })
    ).toBe(false);

    const visible = projectWebSessionVisibleTimelineBlocks([
      thinkingOnly,
      buildMessageBlock('answer', 2),
    ]);
    expect(visible.map(block => block.id)).toEqual(['answer']);
  });

  it('folds adjacent Pi tool rows by adjacency but keeps other agents kind-keyed', () => {
    const bash: WebSessionBlock = {
      key: 'bash-1',
      id: 'bash-1',
      orderIndex: 1,
      kind: 'tool',
      itemType: 'dynamic_tool_call',
      text: '',
      timestamp: Date.UTC(2026, 3, 20, 12, 0, 0),
      attachments: [],
      tool: {
        id: 'bash-1',
        name: 'bash',
        kind: 'dynamic_tool_call',
        status: 'done',
        output: 'ok',
      },
    };
    const note: WebSessionBlock = {
      ...bash,
      key: 'note-1',
      id: 'note-1',
      orderIndex: 2,
      tool: { ...bash.tool!, id: 'note-1', name: 'ctx_note', output: 'saved' },
    };

    // Codex and Claude Code group by kind/name, so different dynamic tools stay apart.
    expect(projectWebSessionCompactTimelineBlocks([bash, note]).map(block => block.key)).toEqual([
      'bash-1',
      'note-1',
    ]);

    // Pi folds by adjacency instead, anchored on the first row of the run.
    const folded = projectWebSessionCompactTimelineBlocks([bash, note], 'pi');
    expect(folded).toHaveLength(1);
    expect(folded[0].tool?.name).toBe('ctx_note');
    expect(folded[0].tool?.commandGroup?.count).toBe(2);
    expect(folded[0].orderIndex).toBe(1);
  });

  it('reuses the group id the server stamped on a Pi activity group', () => {
    const group = { id: 'cmdgrp_bash-1', count: 1, compacted: true };
    const base: WebSessionBlock = {
      key: 'a',
      id: 'a',
      orderIndex: 1,
      kind: 'tool',
      itemType: 'dynamic_tool_call',
      text: '',
      timestamp: Date.UTC(2026, 3, 20, 12, 0, 0),
      attachments: [],
      tool: {
        id: 'a',
        name: 'bash',
        kind: 'dynamic_tool_call',
        status: 'done',
        output: '',
        commandGroup: group,
      },
    };
    const second: WebSessionBlock = {
      ...base,
      key: 'b',
      id: 'b',
      orderIndex: 2,
      tool: { ...base.tool!, id: 'b', name: 'ctx_note', commandGroup: { ...group, count: 2 } },
    };

    const folded = projectWebSessionCompactTimelineBlocks([base, second], 'pi');
    expect(folded).toHaveLength(1);
    expect(folded[0].key).toBe('compact-tool:cmdgrp_bash-1');
    expect(folded[0].tool?.commandGroup?.compacted).toBe(true);
  });

  it('folds consecutive file_change blocks that share a command group id', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildMessageBlock('intro', 1),
      buildFileChangeBlock('fc-1', {
        orderIndex: 2,
        path: 'ui/src/App.vue',
        groupId: 'fc-group-1',
        count: 1,
      }),
      buildFileChangeBlock('fc-2', {
        orderIndex: 3,
        path: 'ui/src/components/Panel.vue',
        groupId: 'fc-group-1',
        count: 2,
      }),
      buildMessageBlock('after', 4),
    ]);

    expect(projected).toHaveLength(3);
    expect(projected[1].tool?.commandGroup?.id).toBe('fc-group-1');
    expect(projected[1].tool?.commandGroup?.count).toBe(2);
    expect(projected[1].tool?.meta?.subtitle).toBe('ui/src/components/Panel.vue');

    const groupItems = readGroupItems(projected[1]);
    expect(groupItems).toHaveLength(2);
    expect(groupItems.map(item => item.summary)).toEqual([
      'ui/src/App.vue',
      'ui/src/components/Panel.vue',
    ]);
  });

  it('builds a synthetic compact group for consecutive ungrouped file changes', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-a', {
        orderIndex: 1,
        path: 'ui/src/a.ts',
      }),
      buildFileChangeBlock('fc-b', {
        orderIndex: 2,
        path: 'ui/src/b.ts',
      }),
    ]);

    expect(projected).toHaveLength(1);
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(projected[0].tool?.commandGroup?.id).toContain('timeline-file-change:');

    const groupItems = readGroupItems(projected[0]);
    expect(groupItems).toHaveLength(2);
    expect(groupItems[0].summary).toBe('ui/src/a.ts');
    expect(groupItems[1].summary).toBe('ui/src/b.ts');
  });

  it('folds consecutive file_change blocks when stale group ids differ', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-1', {
        orderIndex: 1,
        path: 'ui/src/App.vue',
        groupId: 'fc-group-1',
      }),
      buildFileChangeBlock('fc-2', {
        orderIndex: 2,
        path: 'ui/src/components/Panel.vue',
        groupId: 'fc-group-2',
      }),
    ]);

    expect(projected).toHaveLength(1);
    expect(projected[0].tool?.commandGroup?.id).toBe('fc-group-1');
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(readGroupItems(projected[0]).map(item => item.summary)).toEqual([
      'ui/src/App.vue',
      'ui/src/components/Panel.vue',
    ]);
  });

  it('folds consecutive command execution blocks when stale group ids differ', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildCommandBlock('cmd-1', {
        orderIndex: 1,
        command: 'git status',
        groupId: 'cmd-group-1',
      }),
      buildCommandBlock('cmd-2', {
        orderIndex: 2,
        command: 'git diff',
        groupId: 'cmd-group-2',
      }),
    ]);

    expect(projected).toHaveLength(1);
    expect(projected[0].tool?.commandGroup?.id).toBe('cmd-group-1');
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(readGroupItems(projected[0]).map(item => item.command)).toEqual([
      'git status',
      'git diff',
    ]);
  });

  it('folds dynamic tools by name and keeps different built-ins separate', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildDynamicToolBlock('read-1', 'Read', { file_path: 'src/App.vue' }, { orderIndex: 1 }),
      buildDynamicToolBlock('read-2', 'Read', { file_path: 'src/main.ts' }, { orderIndex: 2 }),
      buildDynamicToolBlock(
        'grep-1',
        'Grep',
        { pattern: 'WebSessionPanel', path: 'ui/src' },
        { orderIndex: 3 }
      ),
    ]);

    expect(projected).toHaveLength(2);
    expect(projected[0].tool?.name).toBe('Read');
    expect(projected[0].tool?.commandGroup?.count).toBe(2);
    expect(readGroupItems(projected[0]).map(item => item.summary)).toEqual([
      'src/App.vue',
      'src/main.ts',
    ]);
    expect(projected[1].tool?.name).toBe('Grep');
    expect(projected[1].tool?.commandGroup).toBeUndefined();
    expect(projected[1].tool?.meta?.title).toBe('Grep');
  });

  it('uses dynamic tool targets as compact summaries', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildDynamicToolBlock('grep-1', 'Grep', {
        pattern: 'dynamic_tool_call',
        path: 'service/websession',
      }),
      buildDynamicToolBlock('grep-2', 'Grep', {
        pattern: 'AskUserQuestion',
        path: 'service/websession/manager.go',
      }),
    ]);

    const groupItems = readGroupItems(projected[0]);
    expect(groupItems.map(item => item.summary)).toEqual([
      'dynamic_tool_call · service/websession',
      'AskUserQuestion · service/websession/manager.go',
    ]);
  });

  it('does not merge compact tool blocks across non-tool blocks or different kinds', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-a', {
        orderIndex: 1,
        path: 'ui/src/a.ts',
      }),
      buildMessageBlock('split', 2),
      buildFileChangeBlock('fc-b', {
        orderIndex: 3,
        path: 'ui/src/b.ts',
      }),
      buildFileChangeBlock('fc-c', {
        orderIndex: 4,
        path: 'ui/src/c.ts',
        groupId: 'fc-group-c',
      }),
      buildCommandBlock('cmd-a', {
        orderIndex: 5,
        command: 'git status',
      }),
    ]);

    expect(projected).toHaveLength(4);
    expect(projected[0].tool?.commandGroup).toBeUndefined();
    expect(projected[2].tool?.commandGroup?.id).toBe('fc-group-c');
    expect(projected[2].tool?.commandGroup?.count).toBe(2);
    expect(projected[3].tool?.kind).toBe('command_execution');
  });

  it('does not merge adjacent compact tools from different agent threads', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildCommandBlock('agent-a-command', {
        orderIndex: 1,
        command: 'git status',
        sourceThreadId: 'thread-agent-a',
      }),
      buildCommandBlock('agent-b-command', {
        orderIndex: 2,
        command: 'git diff',
        sourceThreadId: 'thread-agent-b',
      }),
    ]);

    expect(projected).toHaveLength(2);
    expect(projected[0].sourceThreadId).toBe('thread-agent-a');
    expect(projected[1].sourceThreadId).toBe('thread-agent-b');
  });

  it('deduplicates merged group detail items by tool id while keeping the latest state', () => {
    const projected = projectWebSessionCompactTimelineBlocks([
      buildFileChangeBlock('fc-1', {
        orderIndex: 1,
        groupId: 'fc-group-1',
        path: 'ui/src/App.vue',
        groupItems: [
          {
            toolId: 'fc-1',
            kind: 'file_change',
            title: 'FileChange',
            summary: 'ui/src/App.vue',
            command: 'ui/src/App.vue',
            status: 'running',
            timestamp: '2026-04-20T12:00:00.000Z',
          },
        ],
      }),
      buildFileChangeBlock('fc-2', {
        orderIndex: 2,
        groupId: 'fc-group-1',
        count: 2,
        path: 'ui/src/components/Panel.vue',
        groupItems: [
          {
            toolId: 'fc-1',
            kind: 'file_change',
            title: 'FileChange',
            summary: 'ui/src/App.vue',
            command: 'ui/src/App.vue',
            status: 'done',
            timestamp: '2026-04-20T12:00:00.000Z',
          },
          {
            toolId: 'fc-2',
            kind: 'file_change',
            title: 'FileChange',
            summary: 'ui/src/components/Panel.vue',
            command: 'ui/src/components/Panel.vue',
            status: 'done',
            timestamp: '2026-04-20T12:00:01.000Z',
          },
        ],
      }),
    ]);

    const groupItems = readGroupItems(projected[0]);
    expect(groupItems).toHaveLength(2);
    expect(groupItems.map(item => item.toolId)).toEqual(['fc-1', 'fc-2']);
    expect(groupItems[0].status).toBe('done');
  });
});
