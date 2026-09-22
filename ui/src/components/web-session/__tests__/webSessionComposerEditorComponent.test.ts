// @vitest-environment happy-dom

import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { describe, expect, it } from 'vitest';

import WebSessionComposerEditor from '@/components/web-session/WebSessionComposerEditor.vue';

async function flushEditor() {
  await nextTick();
  await new Promise(resolve => setTimeout(resolve, 0));
  await nextTick();
}

function pasteEvent(plainText: string, html = '') {
  const event = new Event('paste', { bubbles: true, cancelable: true });
  Object.defineProperty(event, 'clipboardData', {
    value: {
      getData: (type: string) => (type === 'text/plain' ? plainText : html),
    },
  });
  return event;
}

describe('WebSessionComposerEditor', () => {
  it('keeps restored text without emitting an update when editing is unlocked', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'restored draft',
        disabled: true,
      },
    });
    await nextTick();
    await nextTick();

    const editor = wrapper.get('[role="textbox"]');
    expect(editor.text()).toBe('restored draft');
    expect(editor.attributes('contenteditable')).toBe('false');
    expect(editor.attributes('aria-disabled')).toBe('true');

    await wrapper.setProps({ disabled: false });
    await nextTick();

    expect(editor.text()).toBe('restored draft');
    expect(editor.attributes('contenteditable')).toBe('true');
    expect(editor.attributes('aria-disabled')).toBe('false');
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
  });

  it('keeps every paragraph when native DOM input creates multiple paragraphs', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'first',
      },
    });
    await nextTick();
    await nextTick();

    const surface = wrapper.get('[role="textbox"]').element as HTMLElement;
    surface.innerHTML = '<p>first</p><p>second</p><p>third</p>';
    surface.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await nextTick();
    await new Promise(resolve => setTimeout(resolve, 0));
    await nextTick();

    expect(surface.innerHTML).toContain('<p>second</p>');
    expect(surface.innerHTML).toContain('<p>third</p>');
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('first\nsecond\nthird');
  });

  it('inserts a multiline plain-text paste exactly once and keeps the cursor after it', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'before after',
      },
    });
    await flushEditor();

    const exposed = wrapper.vm as unknown as {
      getSelectionRange: () => { start: number; end: number };
      setSelectionRange: (start: number, end?: number) => void;
    };
    exposed.setSelectionRange(7, 12);
    const pastedText = '\r\n  first\r\n\n\tsecond\r\n';
    const normalizedPastedText = pastedText.replace(/\r\n?/g, '\n');
    const event = pasteEvent(pastedText);
    wrapper.get('[role="textbox"]').element.dispatchEvent(event);
    await flushEditor();

    expect(event.defaultPrevented).toBe(true);
    const updates = wrapper.emitted('update:modelValue') ?? [];
    expect(updates.at(-1)?.[0]).toBe(`before ${normalizedPastedText}`);
    expect(updates).toHaveLength(1);
    expect(exposed.getSelectionRange()).toEqual({
      start: 7 + normalizedPastedText.length,
      end: 7 + normalizedPastedText.length,
    });
  });

  it('does not consume an empty text payload and lets HTML-only paste use ProseMirror parsing', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'keep me',
      },
    });
    await flushEditor();

    const exposed = wrapper.vm as unknown as {
      setSelectionRange: (start: number, end?: number) => void;
    };
    exposed.setSelectionRange(0, 4);
    const emptyEvent = pasteEvent('');
    wrapper.get('[role="textbox"]').element.dispatchEvent(emptyEvent);
    await flushEditor();
    expect(emptyEvent.defaultPrevented).toBe(false);
    expect(wrapper.get('[role="textbox"]').text()).toBe('keep me');
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();

    const missingClipboardEvent = new Event('paste', { bubbles: true, cancelable: true });
    wrapper.get('[role="textbox"]').element.dispatchEvent(missingClipboardEvent);
    await flushEditor();
    expect(missingClipboardEvent.defaultPrevented).toBe(false);
    expect(wrapper.get('[role="textbox"]').text()).toBe('keep me');

    exposed.setSelectionRange(0, 7);
    const htmlEvent = pasteEvent('', '<p>first</p><p>second</p><p>third</p>');
    wrapper.get('[role="textbox"]').element.dispatchEvent(htmlEvent);
    await flushEditor();
    expect(htmlEvent.defaultPrevented).toBe(true);
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('first\nsecond\nthird');
  });

  it('preserves a large multiline paste, cross-paragraph replacement, and consecutive paste', async () => {
    const largeText = Array.from(
      { length: 5000 },
      (_, index) => `line ${index}\n\t  code ${index}`
    ).join('\n');
    expect(largeText.length).toBeGreaterThan(50_000);

    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'a\nb\nc',
      },
    });
    await flushEditor();
    const surface = wrapper.get('[role="textbox"]').element;
    surface.innerHTML = '<p>a</p><p>b</p><p>c</p>';
    surface.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await flushEditor();
    const exposed = wrapper.vm as unknown as {
      getSelectionRange: () => { start: number; end: number };
      setSelectionRange: (start: number, end?: number) => void;
    };

    exposed.setSelectionRange(2, 5);
    surface.dispatchEvent(pasteEvent('B\nC'));
    await flushEditor();
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('a\nB\nC');
    expect(exposed.getSelectionRange()).toEqual({ start: 5, end: 5 });

    surface.dispatchEvent(pasteEvent('!'));
    await flushEditor();
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('a\nB\nC!');

    exposed.setSelectionRange(0);
    surface.dispatchEvent(pasteEvent(largeText));
    await flushEditor();
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe(`${largeText}a\nB\nC!`);
  });

  it('keeps paste changes in the undo and redo history', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'one',
      },
    });
    await flushEditor();

    const exposed = wrapper.vm as unknown as {
      setSelectionRange: (start: number, end?: number) => void;
    };
    exposed.setSelectionRange(3);
    const surface = wrapper.get('[role="textbox"]').element;
    surface.dispatchEvent(pasteEvent(' two'));
    await flushEditor();
    expect(surface.textContent).toBe('one two');

    surface.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true })
    );
    await flushEditor();
    expect(surface.textContent).toBe('one');

    surface.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'y', ctrlKey: true, bubbles: true, cancelable: true })
    );
    await flushEditor();
    expect(surface.textContent).toBe('one two');
  });

  it('does not overwrite a committed IME value with a stale model update', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: '',
      },
    });
    await flushEditor();

    const surface = wrapper.get('[role="textbox"]').element as HTMLElement;
    surface.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    await wrapper.setProps({ modelValue: 'stale external value' });
    await nextTick();
    surface.innerHTML = '<p>中文输入</p>';
    surface.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await flushEditor();
    surface.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    await flushEditor();

    expect(surface.textContent).toBe('中文输入');
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('中文输入');
  });

  it('keeps command highlights on their actual native paragraphs', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'placeholder',
        skills: [
          {
            name: 'openai-docs',
            displayName: 'OpenAI Docs',
            description: '',
            defaultPrompt: '',
            source: 'user',
          },
        ],
      },
    });
    await flushEditor();

    const surface = wrapper.get('[role="textbox"]').element as HTMLElement;
    surface.innerHTML = '<p>/goal first</p><p>$openai-docs</p><p>/compact keep</p>';
    surface.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await flushEditor();

    expect(surface.querySelector('.composer-goal-command')?.textContent).toBe('/goal');
    expect(surface.querySelector('.composer-skill-token')?.textContent).toBe('$openai-docs');
    expect(surface.querySelector('.composer-compact-command')?.textContent).toBe('/compact');
  });

  it('applies a skill completion in a later paragraph and leaves the cursor after the token', async () => {
    const wrapper = mount(WebSessionComposerEditor, {
      props: {
        modelValue: 'first line\nsecond line\n$open',
        skills: [
          {
            name: 'openai-docs',
            displayName: 'OpenAI Docs',
            description: '',
            defaultPrompt: '',
            source: 'user',
          },
        ],
      },
    });
    await flushEditor();

    const exposed = wrapper.vm as unknown as {
      getSelectionRange: () => { start: number; end: number };
      setSelectionRange: (start: number, end?: number) => void;
    };
    exposed.setSelectionRange('first line\nsecond line\n$open'.length);
    await flushEditor();
    const option = wrapper.find('.web-session-composer-editor__completion-option');
    expect(option.exists()).toBe(true);
    expect(option.text()).toContain('$openai-docs');
    await option.trigger('mousedown');
    await flushEditor();

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe(
      'first line\nsecond line\n$openai-docs'
    );
    expect(exposed.getSelectionRange()).toEqual({
      start: 'first line\nsecond line\n$openai-docs'.length,
      end: 'first line\nsecond line\n$openai-docs'.length,
    });
  });
});
