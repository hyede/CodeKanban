// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';

import WebSessionStreamingMarkdown from '../WebSessionStreamingMarkdown.vue';

function mountStreaming(blocks: Array<{ key: string; html: string }>) {
  return mount(WebSessionStreamingMarkdown, { props: { blocks } });
}

describe('WebSessionStreamingMarkdown component', () => {
  it('renders one wrapper per block and keeps markup intact', () => {
    const wrapper = mountStreaming([
      { key: '0', html: '<h1>Title</h1>' },
      { key: '1', html: '<p>first</p>' },
    ]);

    expect(wrapper.findAll('.streaming-markdown-block')).toHaveLength(2);
    expect(wrapper.find('h1').text()).toBe('Title');
    expect(wrapper.text()).toContain('first');
  });

  it('keeps settled DOM nodes while the tail block grows', async () => {
    const wrapper = mountStreaming([
      { key: '0', html: '<p>settled</p>' },
      { key: '1', html: '<p>growing' },
    ]);
    const settledElement = wrapper.findAll('.streaming-markdown-block')[0].element;

    await wrapper.setProps({
      blocks: [
        { key: '0', html: '<p>settled</p>' },
        { key: '1', html: '<p>growing more</p>' },
      ],
    });

    expect(wrapper.findAll('.streaming-markdown-block')[0].element).toBe(settledElement);
    expect(wrapper.text()).toContain('growing more');
  });

  it('patches a block whose html changed and appends new blocks', async () => {
    const wrapper = mountStreaming([{ key: '0', html: '<p>one</p>' }]);

    await wrapper.setProps({
      blocks: [
        { key: '0', html: '<p>one</p>' },
        { key: '1', html: '<pre><code>tail</code></pre>' },
      ],
    });

    expect(wrapper.findAll('.streaming-markdown-block')).toHaveLength(2);
    expect(wrapper.find('code').text()).toBe('tail');

    await wrapper.setProps({ blocks: [{ key: '0', html: '<p>rewritten</p>' }] });

    expect(wrapper.findAll('.streaming-markdown-block')).toHaveLength(1);
    expect(wrapper.text()).toContain('rewritten');
  });

  it('renders nothing for an empty block list', () => {
    const wrapper = mountStreaming([]);

    expect(wrapper.findAll('.streaming-markdown-block')).toHaveLength(0);
    expect(wrapper.find('.streaming-markdown').exists()).toBe(true);
  });
});
