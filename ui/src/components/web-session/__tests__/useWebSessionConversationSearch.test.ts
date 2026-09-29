// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, nextTick, ref } from 'vue';
import { flushPromises, mount } from '@vue/test-utils';
import { webSessionApi } from '@/api/webSession';
import type { WebSessionBlock } from '@/stores/webSession';
import { renderMarkdown } from '@/utils/markdown';
import { useWebSessionConversationSearch } from '../useWebSessionConversationSearch';

vi.mock('@/api/webSession', () => ({ webSessionApi: { searchConversation: vi.fn() } }));

const cleanups: Array<() => void> = [];
afterEach(() => {
  cleanups.splice(0).forEach(cleanup => cleanup());
  vi.useRealTimers();
  vi.resetAllMocks();
  document.body.replaceChildren();
});

function block(id: string, text: string, orderIndex = 1): WebSessionBlock {
  return {
    id,
    key: id,
    text,
    orderIndex,
    kind: 'assistant',
    itemType: 'assistant_message',
    timestamp: 1,
    attachments: [],
  };
}

function harness(initialBlocks = [block('message', 'plan **PLAN** and plan again')]) {
  const blocks = ref(initialBlocks);
  const scroll = vi.fn();
  const loadHistory = vi.fn(async () => false);
  let search!: ReturnType<typeof useWebSessionConversationSearch>;
  const wrapper = mount(
    defineComponent({
      setup() {
        search = useWebSessionConversationSearch({
          currentSession: ref({ id: 'session', projectId: 'project' }),
          visibleBlocks: blocks,
          allBlocks: blocks,
          isActive: true,
          translate: key => key,
          onOpen: () => {},
          loadEarlierHistory: loadHistory,
          scrollToBlock: scroll,
          getBlockElement: key => document.getElementById(key) ?? undefined,
        });
        return () =>
          h('div', [
            h('input', { onKeydown: search.handleInputKeydown }),
            h('span', search.resultLabel.value),
            ...blocks.value.map(item =>
              h('article', {
                id: item.key,
                innerHTML: renderMarkdown(item.text, {
                  textHighlightQuery: search.getBlockQuery(item),
                }),
              })
            ),
          ]);
      },
    }),
    { attachTo: document.body }
  );
  cleanups.push(() => wrapper.unmount());
  search.open();
  search.query.value = 'plan';
  return { search, blocks, wrapper, scroll, loadHistory };
}

describe('conversation search occurrence navigation', () => {
  it('visits every occurrence and wraps without leaving multiple active marks', async () => {
    const { search, wrapper, scroll } = harness();
    await flushPromises();
    const marks = wrapper.findAll('mark');
    expect(search.matches.value).toHaveLength(3);
    expect(search.resultLabel.value).toBe('3 / 3');
    expect(scroll).toHaveBeenLastCalledWith('message', marks[2]!.element);
    for (const [direction, index] of [
      ['previous', 1],
      ['next', 2],
      ['next', 0],
      ['previous', 2],
    ] as const) {
      await search.navigate(direction);
      expect(search.currentIndex.value).toBe(index);
      expect(wrapper.findAll('[data-search-active]')).toHaveLength(1);
      expect(marks[index]!.attributes('aria-current')).toBe('true');
      expect(scroll).toHaveBeenLastCalledWith('message', marks[index]!.element);
    }
  });

  it('supports Enter, Shift+Enter and Escape while ignoring IME confirmation', async () => {
    const { search, wrapper } = harness();
    await flushPromises();
    await wrapper.find('input').trigger('keydown', { key: 'Enter', shiftKey: true });
    expect(search.currentIndex.value).toBe(1);
    await wrapper.find('input').trigger('keydown', { key: 'Enter', isComposing: true });
    expect(search.currentIndex.value).toBe(1);
    await wrapper.find('input').trigger('keydown', { key: 'Enter' });
    expect(search.currentIndex.value).toBe(2);
    await wrapper.find('input').trigger('keydown', { key: 'Escape' });
    expect(search.openState.value).toBe(false);
    expect(wrapper.findAll('mark')).toHaveLength(0);
  });

  it('restores the active occurrence after streaming replaces its HTML', async () => {
    const { search, blocks, wrapper } = harness();
    await flushPromises();
    await search.navigate('previous');
    blocks.value = [block('message', 'plan **PLAN** and plan again; more plan')];
    await nextTick();
    expect(search.matches.value).toHaveLength(4);
    expect(search.currentIndex.value).toBe(1);
    expect(wrapper.findAll('mark')[1]!.attributes('data-search-active')).toBe('true');
    expect(wrapper.findAll('[data-search-active]')).toHaveLength(1);
  });

  it('keeps the selected occurrence when older remote matches arrive', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    vi.mocked(webSessionApi.searchConversation).mockResolvedValue({
      items: [{ id: 'older', kind: 'user', orderIndex: 0, text: 'plan plan' }],
      done: true,
      total: 1,
    });
    const { search, wrapper } = harness();
    await flushPromises();
    await search.navigate('previous');
    await vi.advanceTimersByTimeAsync(3000);
    expect(search.resultLabel.value).toBe('4 / 5');
    expect(wrapper.findAll('mark')[1]!.attributes('data-search-active')).toBe('true');
  });

  it('counts unloaded occurrences and keeps the selected one after loading history', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    vi.mocked(webSessionApi.searchConversation).mockResolvedValue({
      items: [{ id: 'older', kind: 'assistant', orderIndex: 0, text: 'plan plan' }],
      done: true,
      total: 1,
    });
    const { search, blocks, wrapper, loadHistory, scroll } = harness([]);
    loadHistory.mockImplementation(async () => {
      blocks.value = [block('older', 'plan plan', 0)];
      return true;
    });
    await vi.advanceTimersByTimeAsync(3000);
    await flushPromises();
    expect(search.resultLabel.value).toBe('2 / 2');
    expect(scroll).toHaveBeenLastCalledWith('older', wrapper.findAll('mark')[1]!.element);
    await search.navigate('previous');
    expect(scroll).toHaveBeenLastCalledWith('older', wrapper.findAll('mark')[0]!.element);
  });

  it('cancels pending history navigation when the search closes', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    vi.mocked(webSessionApi.searchConversation).mockResolvedValue({
      items: [{ id: 'older', kind: 'assistant', orderIndex: 0, text: 'plan plan' }],
      done: true,
      total: 1,
    });
    const { search, blocks, loadHistory, scroll } = harness([]);
    let finish!: (value: boolean) => void;
    loadHistory.mockImplementation(
      () =>
        new Promise(resolve => {
          finish = resolve;
        })
    );
    await vi.advanceTimersByTimeAsync(3000);
    expect(loadHistory).toHaveBeenCalledOnce();
    search.close();
    blocks.value = [block('older', 'plan plan', 0)];
    finish(true);
    await flushPromises();
    expect(scroll).not.toHaveBeenCalled();
    expect(search.matches.value).toHaveLength(0);
  });

  it('removes highlights and occurrences when their role is filtered out', async () => {
    const { search, wrapper } = harness();
    await flushPromises();
    search.filters.value.assistant = false;
    await flushPromises();
    expect(search.matches.value).toHaveLength(0);
    expect(wrapper.findAll('mark')).toHaveLength(0);
  });

  it('keeps a remote metadata match on its compacted tool card', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    vi.mocked(webSessionApi.searchConversation).mockResolvedValue({
      items: [
        {
          id: 'old-tool',
          toolId: 'old-tool',
          commandGroupId: 'group',
          kind: 'tool',
          orderIndex: 0,
        },
      ],
      done: true,
      total: 1,
    });
    const { search, scroll } = harness([
      {
        ...block('latest-tool', ''),
        kind: 'tool',
        tool: {
          id: 'latest-tool',
          name: 'CommandExecution',
          status: 'done',
          commandGroup: { id: 'group', count: 2 },
        },
      },
    ]);
    search.filters.value.tools = true;
    await vi.advanceTimersByTimeAsync(3000);
    await flushPromises();
    expect(search.matches.value).toHaveLength(1);
    expect(scroll).toHaveBeenLastCalledWith('latest-tool', undefined);
  });
});
