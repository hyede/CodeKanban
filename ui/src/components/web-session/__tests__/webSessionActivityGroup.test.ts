// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';

import WebSessionActivityGroup from '../WebSessionActivityGroup.vue';
import type { PiActivityGroupRow } from '@/components/web-session/webSessionPiActivityGroup';

const rows: PiActivityGroupRow[] = [
  { key: 'bash-1', name: 'bash', summary: 'npm warn Unknown user config', note: '', body: 'out-1' },
  { key: 'rtk', name: '', summary: '', note: 'RTK rewrite: cat a -> rtk read a', body: '' },
  { key: 'bash-2', name: 'bash', summary: 'ui/src/App.vue', note: '', body: 'out-2' },
];

function mountGroup(expanded = false, streaming = false) {
  return mount(WebSessionActivityGroup, {
    props: {
      rows,
      label: streaming ? '处理中' : '已处理',
      stepsLabel: '2 步',
      summary: 'ui/src/App.vue',
      time: '21:57:03',
      timeTitle: '2026-09-10 21:57:03',
      expanded,
      streaming,
    },
  });
}

describe('WebSessionActivityGroup', () => {
  it('collapses to a single low-key header line', () => {
    const wrapper = mountGroup();
    expect(wrapper.get('.activity-group-header').attributes('aria-expanded')).toBe('false');
    expect(wrapper.text()).toContain('已处理');
    expect(wrapper.text()).toContain('2 步');
    expect(wrapper.text()).toContain('ui/src/App.vue');
    expect(wrapper.text()).toContain('21:57:03');
    expect(wrapper.find('.activity-group-body').exists()).toBe(false);
    expect(wrapper.find('.activity-group-dot').exists()).toBe(false);
  });

  it('shows a pulsing marker while the run is still streaming', () => {
    const wrapper = mountGroup(true, true);
    expect(wrapper.find('.activity-group-dot').exists()).toBe(true);
    expect(wrapper.text()).toContain('处理中');
  });

  it('lists one summary row per step and folds notes into a dim line', () => {
    const wrapper = mountGroup(true);
    expect(wrapper.findAll('.activity-group-row')).toHaveLength(2);
    expect(wrapper.findAll('.activity-group-note')).toHaveLength(1);
    expect(wrapper.get('.activity-group-note').text()).toContain('RTK rewrite');
    // Row bodies stay collapsed until the reader asks for them.
    expect(wrapper.find('.activity-group-row-body').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('out-1');
  });

  it('reveals one step body at a time without touching the others', async () => {
    const wrapper = mountGroup(true);
    const headers = wrapper.findAll('.activity-group-row-header');
    await headers[0].trigger('click');
    expect(wrapper.get('.activity-group-row-body').text()).toContain('out-1');
    expect(headers[0].attributes('aria-expanded')).toBe('true');

    await wrapper.findAll('.activity-group-row-header')[1].trigger('click');
    expect(wrapper.findAll('.activity-group-row-body')).toHaveLength(2);
    expect(wrapper.text()).toContain('out-2');
  });

  it('emits toggle from the header', async () => {
    const wrapper = mountGroup();
    await wrapper.get('.activity-group-header').trigger('click');
    expect(wrapper.emitted('toggle')).toHaveLength(1);
  });
});
