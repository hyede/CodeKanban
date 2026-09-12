// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { h, nextTick } from 'vue';

import WebSessionReasoningSummary from '../WebSessionReasoningSummary.vue';
import { renderMarkdown } from '@/utils/markdown';

const text = '**Checking inverse ReplaceStep handling**\n\n**Refining ReplaceStep checks**';
const props = {
  text,
  label: '思考摘要',
  time: '18:19:11',
  timeTitle: '2026-09-05 18:19:11',
};

function mountSummary(expanded = false) {
  return mount(WebSessionReasoningSummary, {
    props: { ...props, expanded },
    slots: { default: () => h('div', { innerHTML: renderMarkdown(text) }) },
  });
}

describe('WebSessionReasoningSummary', () => {
  it('starts collapsed with a single label and timestamp', () => {
    const wrapper = mountSummary();
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('false');
    expect(wrapper.text()).toContain('思考摘要');
    expect(wrapper.text().match(/思考摘要/g)).toHaveLength(1);
    expect(wrapper.text()).toContain('18:19:11');
    expect(wrapper.find('.reasoning-summary-body').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('Checking');
  });

  it('toggles and renders separate Markdown paragraphs without tool chrome', async () => {
    const wrapper = mountSummary();
    await wrapper.get('button').trigger('click');
    expect(wrapper.emitted('toggle')).toHaveLength(1);
    await wrapper.setProps({ expanded: true });
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('true');
    expect(wrapper.findAll('p strong').map(node => node.text())).toEqual([
      'Checking inverse ReplaceStep handling',
      'Refining ReplaceStep checks',
    ]);
    expect(wrapper.text()).not.toContain('**');
    expect(wrapper.find('pre').exists()).toBe(false);
    await wrapper.get('button').trigger('click');
    expect(wrapper.emitted('toggle')).toHaveLength(2);
    await wrapper.setProps({ expanded: false });
    expect(wrapper.find('.reasoning-summary-body').exists()).toBe(false);
  });

  it('does not render an empty summary', () => {
    const wrapper = mount(WebSessionReasoningSummary, {
      props: { ...props, text: ' \n ', expanded: true },
    });
    expect(wrapper.find('button').exists()).toBe(false);
    expect(wrapper.text()).toBe('');
  });

  it('preserves expansion on refresh and defaults to collapsed when remounted', async () => {
    const wrapper = mountSummary(true);
    await wrapper.setProps({ text: `${text}\n\nUpdated summary` });
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('true');
    wrapper.unmount();
    const refreshed = mountSummary();
    expect(refreshed.get('button').attributes('aria-expanded')).toBe('false');
  });

  it('shows a live preview and streaming marker for Pi thinking', () => {
    const wrapper = mount(WebSessionReasoningSummary, {
      props: {
        ...props,
        label: '思考中',
        summary: 'Refining ReplaceStep checks',
        streaming: true,
        expanded: false,
      },
    });
    expect(wrapper.get('.reasoning-summary-label').classes()).toContain('is-streaming');
    expect(wrapper.find('.reasoning-summary-dot').exists()).toBe(true);
    expect(wrapper.get('.reasoning-summary-preview').text()).toBe('Refining ReplaceStep checks');
    expect(wrapper.find('.reasoning-summary-body').exists()).toBe(false);
  });

  it('omits the preview and streaming marker for settled summaries', () => {
    const wrapper = mountSummary();
    expect(wrapper.find('.reasoning-summary-preview').exists()).toBe(false);
    expect(wrapper.find('.reasoning-summary-dot').exists()).toBe(false);
  });

  it('renders the expanded body inside a framed scroll container', () => {
    const wrapper = mountSummary(true);
    const body = wrapper.get('.reasoning-summary-body');
    expect(body.find('.reasoning-summary-body-content').exists()).toBe(true);
    expect(body.get('p strong').text()).toBe('Checking inverse ReplaceStep handling');
  });

  it('keeps a streaming body pinned to its tail and lets the reader break the pin', async () => {
    const wrapper = mount(WebSessionReasoningSummary, {
      props: { ...props, expanded: true, streaming: true },
      slots: { default: () => h('div', 'thought body') },
    });
    const bodyEl = wrapper.get('.reasoning-summary-body').element as HTMLElement;
    let scrollTop = 0;
    Object.defineProperty(bodyEl, 'scrollHeight', { value: 900, configurable: true });
    Object.defineProperty(bodyEl, 'clientHeight', { value: 260, configurable: true });
    Object.defineProperty(bodyEl, 'scrollTop', {
      get: () => scrollTop,
      set: (value: number) => {
        scrollTop = value;
      },
      configurable: true,
    });

    await wrapper.setProps({ text: `${text}\n\nMore thinking` });
    await nextTick();
    await nextTick();
    expect(scrollTop).toBe(900);

    // The reader scrolls up to look back: the pin must break and stay broken.
    scrollTop = 120;
    bodyEl.dispatchEvent(new Event('scroll'));
    await wrapper.setProps({ text: `${text}\n\nEven more thinking` });
    await nextTick();
    await nextTick();
    expect(scrollTop).toBe(120);

    wrapper.unmount();
  });

  it('stops following the tail once the stream settles', async () => {
    const wrapper = mount(WebSessionReasoningSummary, {
      props: { ...props, expanded: true, streaming: true },
      slots: { default: () => h('div', 'thought body') },
    });
    const bodyEl = wrapper.get('.reasoning-summary-body').element as HTMLElement;
    let scrollTop = 0;
    Object.defineProperty(bodyEl, 'scrollHeight', { value: 900, configurable: true });
    Object.defineProperty(bodyEl, 'clientHeight', { value: 260, configurable: true });
    Object.defineProperty(bodyEl, 'scrollTop', {
      get: () => scrollTop,
      set: (value: number) => {
        scrollTop = value;
      },
      configurable: true,
    });

    await wrapper.setProps({ streaming: false });
    scrollTop = 40;
    await wrapper.setProps({ text: `${text}\n\nSettled thinking` });
    await nextTick();
    await nextTick();
    expect(scrollTop).toBe(40);

    wrapper.unmount();
  });
});
