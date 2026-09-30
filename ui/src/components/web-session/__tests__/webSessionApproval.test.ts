// @vitest-environment happy-dom

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { computed, defineComponent, reactive, ref, type ComputedRef, type Ref } from 'vue';
import { mount } from '@vue/test-utils';
import ts from 'typescript';
import { describe, expect, it, vi } from 'vitest';
import zhCN from '@/i18n/locales/zh-CN';

const panelSource = readFileSync(resolve(__dirname, '../WebSessionPanel.vue'), 'utf8');
const setupSource = panelSource.split('<script setup lang="ts">')[1]!.split('</script>')[0]!;
const parsedSource = ts.createSourceFile('panel.ts', setupSource, ts.ScriptTarget.Latest, true);
const selectedNames = new Set([
  'pendingApproval',
  'approvalSubmitting',
  'canApproveWithYolo',
  'handleApproval',
]);
const approvalSource = parsedSource.statements
  .filter(statement => {
    if (ts.isFunctionDeclaration(statement)) return selectedNames.has(statement.name?.text ?? '');
    return (
      ts.isVariableStatement(statement) &&
      statement.declarationList.declarations.some(
        declaration => ts.isIdentifier(declaration.name) && selectedNames.has(declaration.name.text)
      )
    );
  })
  .map(statement => statement.getText(parsedSource))
  .join(String.fromCharCode(10));
const compiledSource = ts.transpileModule(approvalSource, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText;
const actionTemplate =
  '<div>' + panelSource.split('<div class="approval-actions">')[1]!.split('</div>')[0] + '</div>';
type Approval = { itemId: string; requestedAt: string; actionable: boolean; stale: boolean };
type ApprovalAction = 'approve' | 'reject' | 'approve_yolo';
type Session = { id: string; agent: string; permissionLevel: string };

function createHarness(agent = 'devin', permissionLevel = 'elevated') {
  const currentRealSession = ref<Session | null>({ id: 'session-1', agent, permissionLevel });
  const approvals = reactive(
    new Map<string, Approval>([
      ['session-1', { itemId: 'tool-1', requestedAt: 'request-1', actionable: true, stale: false }],
    ])
  );
  const webSessionStore = {
    getPendingApproval: vi.fn((sessionId: string) => approvals.get(sessionId) ?? null),
    updatePermissionLevel: vi.fn().mockResolvedValue(undefined),
    approveSession: vi.fn().mockResolvedValue(undefined),
    rejectSession: vi.fn().mockResolvedValue(undefined),
  };
  const message = { info: vi.fn(), error: vi.fn() };
  const dependencies = {
    computed,
    ref,
    currentRealSession,
    webSessionStore,
    message,
    t: (key: string) =>
      key === 'webSession.approvalApproveYolo' ? zhCN.webSession.approvalApproveYolo : key,
    formatSessionInteractionError: (error: Error) => error.message,
  };
  const initialize = new Function(
    ...Object.keys(dependencies),
    compiledSource +
      'return { pendingApproval, approvalSubmitting, canApproveWithYolo, handleApproval };'
  );
  const bindings = initialize(...Object.values(dependencies)) as {
    pendingApproval: ComputedRef<Approval | null>;
    approvalSubmitting: Ref<boolean>;
    canApproveWithYolo: ComputedRef<boolean>;
    handleApproval: (action: ApprovalAction) => Promise<void>;
  };
  return { ...dependencies, ...bindings, approvals };
}

function mountActions(harness: ReturnType<typeof createHarness>) {
  return mount(
    defineComponent({
      setup: () => ({ ...harness, handleAbortCurrent: vi.fn() }),
      template: actionTemplate,
    }),
    {
      global: {
        stubs: {
          NButton: {
            props: ['disabled', 'loading'],
            emits: ['click'],
            template: `<button :disabled="disabled || loading" @click="$emit('click')"><slot /></button>`,
          },
        },
      },
    }
  );
}

describe('Approval mode transition visibility', () => {
  it.each([
    ['devin', 'default', true],
    ['devin', 'elevated', true],
    ['devin', 'yolo', false],
    ['codex', 'default', false],
    ['claude', 'elevated', true],
    ['claude', 'yolo', false],
    ['pi', 'yolo', false],
  ])('shows the option for %s / %s: %s', (agent, level, visible) => {
    const wrapper = mountActions(createHarness(String(agent), String(level)));
    expect(wrapper.text().includes('转为完全自动')).toBe(visible);
    wrapper.unmount();
  });
});

describe.each(['devin', 'claude'])('%s approval mode transition', agent => {
  it('wires the displayed option to changing the mode and continuing', async () => {
    const harness = createHarness(agent);
    const wrapper = mountActions(harness);
    const button = wrapper.findAll('button').find(item => item.text() === '转为完全自动');
    expect(button).toBeDefined();
    await button!.trigger('click');
    expect(harness.webSessionStore.updatePermissionLevel).toHaveBeenCalledWith('session-1', 'yolo');
    expect(harness.webSessionStore.approveSession).toHaveBeenCalledWith('session-1');
    wrapper.unmount();
  });

  it('waits for the mode change and blocks duplicate approval actions', async () => {
    const harness = createHarness(agent);
    let finish!: () => void;
    harness.webSessionStore.updatePermissionLevel.mockImplementation(
      () =>
        new Promise<void>(resolve => {
          finish = resolve;
        })
    );
    const task = harness.handleApproval('approve_yolo');
    expect(harness.approvalSubmitting.value).toBe(true);
    expect(harness.webSessionStore.approveSession).not.toHaveBeenCalled();
    await harness.handleApproval('approve_yolo');
    await harness.handleApproval('reject');
    expect(harness.webSessionStore.updatePermissionLevel).toHaveBeenCalledTimes(1);
    expect(harness.webSessionStore.rejectSession).not.toHaveBeenCalled();
    finish();
    await task;
    expect(harness.webSessionStore.approveSession).toHaveBeenCalledExactlyOnceWith('session-1');
    expect(harness.approvalSubmitting.value).toBe(false);
  });

  it('does not approve after a failed mode change and allows retry', async () => {
    const harness = createHarness(agent);
    harness.webSessionStore.updatePermissionLevel.mockRejectedValueOnce(new Error('mode failed'));
    await harness.handleApproval('approve_yolo');
    expect(harness.webSessionStore.approveSession).not.toHaveBeenCalled();
    expect(harness.message.error).toHaveBeenCalledWith('mode failed');
    expect(harness.approvalSubmitting.value).toBe(false);
    await harness.handleApproval('approve_yolo');
    expect(harness.webSessionStore.approveSession).toHaveBeenCalledOnce();
  });

  it.each(['resolved', 'replaced', 'renewed', 'stale', 'unactionable'])(
    'does not approve a request that becomes %s during the update',
    async change => {
      const harness = createHarness(agent);
      harness.webSessionStore.updatePermissionLevel.mockImplementation(async () => {
        const approval = harness.approvals.get('session-1')!;
        if (change === 'resolved') harness.approvals.delete('session-1');
        if (change === 'replaced') approval.itemId = 'tool-2';
        if (change === 'renewed') approval.requestedAt = 'request-2';
        if (change === 'stale') approval.stale = true;
        if (change === 'unactionable') approval.actionable = false;
      });
      await harness.handleApproval('approve_yolo');
      expect(harness.webSessionStore.approveSession).not.toHaveBeenCalled();
      expect(harness.approvalSubmitting.value).toBe(false);
    }
  );

  it('keeps the action bound to the original session when switching tabs', async () => {
    const harness = createHarness(agent);
    harness.webSessionStore.updatePermissionLevel.mockImplementation(async () => {
      harness.currentRealSession.value = {
        id: 'session-2',
        agent: 'codex',
        permissionLevel: 'default',
      };
      harness.approvals.set('session-2', {
        itemId: 'tool-2',
        requestedAt: 'request-2',
        actionable: true,
        stale: false,
      });
    });
    await harness.handleApproval('approve_yolo');
    expect(harness.webSessionStore.approveSession).toHaveBeenCalledExactlyOnceWith('session-1');
  });

  it.each(['stale', 'unactionable'])('blocks an initially %s approval', async state => {
    const harness = createHarness(agent);
    const approval = harness.approvals.get('session-1')!;
    approval.stale = state === 'stale';
    approval.actionable = state !== 'unactionable';
    const wrapper = mountActions(harness);
    const button = wrapper.findAll('button').find(item => item.text() === '转为完全自动')!;
    expect(button.attributes('disabled')).toBeDefined();
    await harness.handleApproval('approve_yolo');
    expect(harness.webSessionStore.updatePermissionLevel).not.toHaveBeenCalled();
    expect(harness.webSessionStore.approveSession).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('does not change an unsupported agent permission level', async () => {
    const harness = createHarness('codex');
    await harness.handleApproval('approve_yolo');
    expect(harness.webSessionStore.updatePermissionLevel).not.toHaveBeenCalled();
    expect(harness.webSessionStore.approveSession).not.toHaveBeenCalled();
  });

  it.each(['approve', 'reject'] as const)('keeps ordinary %s behavior', async action => {
    const harness = createHarness(agent);
    await harness.handleApproval(action);
    expect(harness.webSessionStore.updatePermissionLevel).not.toHaveBeenCalled();
    const called = action === 'approve' ? 'approveSession' : 'rejectSession';
    expect(harness.webSessionStore[called]).toHaveBeenCalledExactlyOnceWith('session-1');
  });
});
