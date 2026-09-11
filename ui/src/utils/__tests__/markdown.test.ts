import { beforeEach, describe, expect, it } from 'vitest';

import {
  renderHighlightedCodeBlock,
  renderHighlightedPlainText,
  renderMarkdown,
  renderStreamingMarkdownBlocks,
  resetStreamingMarkdownBlocks,
} from '@/utils/markdown';

describe('renderMarkdown', () => {
  it('highlights fenced code blocks by default', () => {
    const html = renderMarkdown('```go\nfmt.Println("hi")\n```');

    expect(html).toContain('class="hljs language-go"');
    expect(html).toContain('data-language="go"');
  });

  it('highlights haxe fenced code blocks', () => {
    const html = renderMarkdown('```haxe\nclass Main {}\n```');

    expect(html).toContain('class="hljs language-haxe"');
    expect(html).toContain('data-language="haxe"');
  });

  it('supports the hx alias for haxe fenced code blocks', () => {
    const html = renderMarkdown('```hx\nclass Main {}\n```');

    expect(html).toContain('class="hljs language-haxe"');
    expect(html).toContain('data-language="hx"');
  });

  it('can skip code highlighting for streaming renders', () => {
    const html = renderMarkdown('```html\n<div class="box">\n```', {
      disableCodeHighlight: true,
    });

    expect(html).toContain('class="hljs"');
    expect(html).toContain('data-language="html"');
    expect(html).not.toContain('language-xml');
    expect(html).not.toContain('<span class="hljs-');
    expect(html).toContain('&lt;div class=&quot;box&quot;&gt;');
  });

  it('adds a code-block copy button only when enabled', () => {
    const enabledHtml = renderMarkdown('```bash\necho "hi"\n```', {
      enableCodeBlockCopy: true,
      codeBlockCopyLabel: 'copy',
    });
    const disabledHtml = renderMarkdown('```bash\necho "hi"\n```');

    expect(enabledHtml).toContain('data-message-code-copy="true"');
    expect(enabledHtml).toContain('class="markdown-code-copy-button"');
    expect(enabledHtml).toContain('>copy</button>');
    expect(enabledHtml).toContain('data-code-copy="true"');
    expect(disabledHtml).not.toContain('data-message-code-copy="true"');
  });

  it('keeps link rendering intact when highlighting is disabled', () => {
    const html = renderMarkdown('[docs](https://example.com)', {
      disableCodeHighlight: true,
    });

    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('data-message-link="true"');
    expect(html).toContain('target="_blank"');
  });

  it('adds a copy button only for absolute http and https links when enabled', () => {
    const html = renderMarkdown(
      [
        '[http](http://10.128.128.111:6032/)',
        '[https](https://example.com/docs)',
        '[relative](/docs/getting-started)',
      ].join('\n\n'),
      {
        enableLinkCopy: true,
        linkCopyLabel: 'Copy link',
      }
    );

    expect(html).toContain('data-message-link-copy="true"');
    expect(html).toContain('data-message-link-copy-href="http://10.128.128.111:6032/"');
    expect(html).toContain('data-message-link-copy-href="https://example.com/docs"');
    expect(html).not.toContain('data-message-link-copy-href="/docs/getting-started"');
  });

  it('does not add copy buttons when link copy is disabled', () => {
    const html = renderMarkdown('[http](http://10.128.128.111:6032/)');

    expect(html).not.toContain('data-message-link-copy="true"');
  });

  it('highlights search text without modifying markdown tags or attributes', () => {
    const html = renderMarkdown('**切换** and [切换](https://example.com)', {
      textHighlightQuery: '切换',
    });

    expect(html).toContain('<strong><mark class="markdown-search-highlight">切换</mark></strong>');
    expect(html).toContain(
      '<a href="https://example.com" target="_blank" rel="noopener noreferrer" data-message-link="true"><mark class="markdown-search-highlight">切换</mark></a>'
    );
    expect(html).not.toContain('href="<mark');
  });

  it('highlights escaped plain text safely', () => {
    const html = renderHighlightedPlainText('<切换> & ready', '<切换>');

    expect(html).toContain('<mark class="markdown-search-highlight">&lt;切换&gt;</mark>');
    expect(html).toContain('&amp; ready');
  });

  it('preserves ordered, unordered, and nested list structure', () => {
    const html = renderMarkdown(
      [
        '1. first',
        '2. second',
        '   - child',
        '   - child two',
        '3. third',
        '   1. nested one',
        '   2. nested two',
      ].join('\n')
    );

    expect(html).toContain('<ol>');
    expect(html).toContain('<ul>');
    expect(html).toContain('<li>first</li>');
    expect(html).toContain('<li>child</li>');
    expect(html).toContain('<li>nested one</li>');
  });

  it('renders standalone highlighted diff blocks', () => {
    const html = renderHighlightedCodeBlock('@@ -1 +1 @@\n-old\n+new\n', 'diff');

    expect(html).toContain('class="hljs language-diff"');
    expect(html).toContain('hljs-addition');
    expect(html).toContain('hljs-deletion');
  });
});

describe('renderMarkdown rendering cache', () => {
  it('reuses the rendered html for an unchanged body', () => {
    const body = '# Heading\n\nsome **body** text';

    // Identity matters: the timeline re-renders on every stream delta, and a
    // fresh parse per render is what made long sessions janky.
    expect(renderMarkdown(body)).toBe(renderMarkdown(body));
  });

  it('re-renders when the body changes', () => {
    const first = renderMarkdown('first body');
    const second = renderMarkdown('second body');

    expect(second).not.toBe(first);
    expect(first).toContain('first body');
    expect(second).toContain('second body');
  });

  it('keeps results apart per highlight query', () => {
    const body = 'searchable body';
    const plain = renderMarkdown(body);
    const highlighted = renderMarkdown(body, { textHighlightQuery: 'searchable' });

    expect(highlighted).not.toBe(plain);
    expect(highlighted).toContain('markdown-search-highlight');
    expect(renderMarkdown(body)).toBe(plain);
  });

  it('drops Magic Context markers before parsing', () => {
    const html = renderMarkdown('§12§ thinking about §13§ the parser');

    expect(html).not.toContain('§');
    expect(html).toContain('thinking about');
  });
});

describe('renderHighlightedPlainText markers', () => {
  it('drops Magic Context markers from raw text', () => {
    expect(renderHighlightedPlainText('§7§ raw §8§ text')).toBe('raw text');
  });
});

describe('renderStreamingMarkdownBlocks', () => {
  beforeEach(() => {
    resetStreamingMarkdownBlocks();
  });

  it('splits a body into its top-level blocks', () => {
    const blocks = renderStreamingMarkdownBlocks(
      'k',
      '# Title\n\nfirst para\n\n```js\nlet a = 1\n```'
    );

    expect(blocks.map(block => block.key)).toEqual(['0', '1', '2']);
    expect(blocks[0].html).toContain('<h1');
    expect(blocks[1].html).toContain('first para');
    expect(blocks[2].html).toContain('hljs');
  });

  it('reuses settled blocks so only the growing tail is re-rendered', () => {
    const first = renderStreamingMarkdownBlocks('k', '# Title\n\nstreaming');
    const second = renderStreamingMarkdownBlocks('k', '# Title\n\nstreaming more text');

    // Object identity is what lets the renderer skip patching: same object
    // means the identical html string, so no DOM work for settled blocks.
    expect(second[0]).toBe(first[0]);
    expect(second[1]).not.toBe(first[1]);
    expect(second[1].html).toContain('more text');
  });

  it('re-renders a block whose source changed', () => {
    const first = renderStreamingMarkdownBlocks('k', 'alpha');
    const second = renderStreamingMarkdownBlocks('k', 'beta');

    expect(second[0]).not.toBe(first[0]);
    expect(second[0].html).toContain('beta');
  });

  it('keeps block state per stream key', () => {
    const a = renderStreamingMarkdownBlocks('stream-a', 'shared text');
    const b = renderStreamingMarkdownBlocks('stream-b', 'shared text');

    expect(b[0].html).toBe(a[0].html);
    expect(b[0]).not.toBe(a[0]);
  });

  it('drops state for an empty body and recovers afterwards', () => {
    renderStreamingMarkdownBlocks('k', 'text');
    expect(renderStreamingMarkdownBlocks('k', '')).toEqual([]);
    expect(renderStreamingMarkdownBlocks('k', 'text')[0].html).toContain('text');
  });

  it('drops Magic Context markers before splitting', () => {
    const blocks = renderStreamingMarkdownBlocks('k', 'drop §12§, §13§ now');

    expect(blocks).toHaveLength(1);
    expect(blocks[0].html).not.toContain('§');
    expect(blocks[0].html).toContain('drop now');
  });
});
