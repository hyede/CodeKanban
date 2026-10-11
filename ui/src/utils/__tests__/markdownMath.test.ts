import { beforeEach, describe, expect, it } from 'vitest';
import {
  renderMarkdown,
  renderStreamingMarkdownBlocks,
  resetStreamingMarkdownBlocks,
} from '@/utils/markdown';

describe('Markdown math', () => {
  it.each(['$x_i^2$', '\\(x_i^2\\)'])('renders inline math: %s', source => {
    const html = renderMarkdown('Value: ' + source + '.');
    expect(html).toContain('class="katex"');
    expect(html).toContain('<msubsup>');
    expect(html).not.toContain('katex-display');
    expect(html).not.toContain('<em>');
  });

  it.each([
    ['\\[', '\\]'],
    ['$$', '$$'],
  ])('renders a multiline fraction: %s', (open, close) => {
    const source = [
      '可以把它理解为：',
      open,
      'E(x,n)\\approx',
      '\\frac{\\sum_i w_i(x,n)v_i(x)E_i(n)}',
      '{\\sum_i w_i(x,n)v_i(x)}',
      close,
      '其中权重控制影响。',
    ].join('\n');
    const html = renderMarkdown(source);
    expect(html).toContain('class="katex-display"');
    expect(html).toContain('<mfrac>');
    expect(html).toContain('可以把它理解为：');
    expect(html).toContain('其中权重控制影响。');
    expect(html).not.toContain('<em>');
    expect(html).not.toContain('markdown-math-source');
  });

  it.each(['before $$x^2$$ after', 'before \\[x^2\\] after'])(
    'supports display math inside a paragraph: %s',
    source => {
      const html = renderMarkdown(source);
      expect(html).toContain('class="katex-display"');
      expect(html).toContain('before');
      expect(html).toContain('after');
      expect(html).not.toContain('<p>before <div');
    }
  );

  it('renders math in lists, blockquotes, and tables', () => {
    const html = renderMarkdown('- $a_b$\n\n> \\(c_d\\)\n\n| Value |\n| --- |\n| $e_f$ |');
    expect(html.match(/class="katex"/g)).toHaveLength(3);
    expect(html).toContain('<ul>');
    expect(html).toContain('<blockquote>');
    expect(html).toContain('<table>');
  });

  it('leaves inline, fenced, and indented code untouched', () => {
    const tick = String.fromCharCode(96);
    const fence = tick.repeat(3);
    const source = [
      tick + '$x$ \\(y\\)' + tick,
      '',
      fence + 'text',
      '\\[x_i\\]',
      '$$y$$',
      fence,
      '',
      '    $$z$$',
    ].join('\n');
    const html = renderMarkdown(source);
    expect(html).not.toContain('class="katex');
    expect(html).toContain('$x$ \\(y\\)');
    expect(html).toContain('\\[x_i\\]');
    expect(html).toContain('$$z$$');
  });

  it.each([
    'Costs $5 and $10.',
    'Costs $5.00, then $10.00.',
    'An unmatched $ stays visible.',
    '$ spaced $ and $trailing $',
    '\\$x\\$ and \\\\(y\\\\)',
    '[label](https://example.com/$path$)',
  ])('does not treat currency, escapes, or link destinations as math: %s', source => {
    const html = renderMarkdown(source);
    expect(html).not.toContain('class="katex');
  });

  it('supports escaped dollar signs inside braces', () => {
    expect(renderMarkdown('$\\text{cost \\$5} + x$')).toContain('class="katex"');
  });

  it('does not close a formula on a delimiter inside braces', () => {
    const html = renderMarkdown('\\[\\text{literal \\]} + x\\]');
    expect(html).toContain('>\\[\\text{literal \\]} + x\\]</span>');
  });

  it('falls back to escaped source for malformed TeX', () => {
    const html = renderMarkdown(
      'Before\n\n$$\\unknowncommand{1} <script>alert(1)</script>$$\n\n**After**'
    );
    expect(html).toContain('markdown-math-source');
    expect(html).toContain('&lt;script&gt;');
    expect(html).not.toContain('<script>');
    expect(html).toContain('<strong>After</strong>');
  });

  it('disables trusted TeX commands and bounds recursive macros', () => {
    const html = renderMarkdown('\\[\\href{javascript:alert(1)}{click}\\]');
    expect(html).not.toContain('href="javascript:');
    expect(renderMarkdown('$$\\def\\loop{\\loop}\\loop$$')).toContain('markdown-math-source');
  });

  it('highlights surrounding text without modifying rendered math', () => {
    const formula = renderMarkdown('$x$').trim().slice(3, -4);
    const html = renderMarkdown('x $x$ x', { textHighlightQuery: 'x' });
    expect(html).toContain(formula);
    expect(html.match(/class="markdown-search-highlight"/g)).toHaveLength(2);
  });
});

describe('streaming Markdown math', () => {
  beforeEach(resetStreamingMarkdownBlocks);

  it.each([
    ['\\[', '\\]'],
    ['$$', '$$'],
  ])('keeps partial math visible until its closing delimiter: %s', (open, close) => {
    const partial = '# Heading\n\n' + open + '\n\\frac{a_b}{c_d}';
    const before = renderStreamingMarkdownBlocks('math', partial);
    const complete = partial + '\n' + close + '\n\nTail';
    const after = renderStreamingMarkdownBlocks('math', complete);
    expect(before).toHaveLength(2);
    expect(before[1]!.html).toContain('markdown-math-source');
    expect(before[1]!.html).toContain('\\frac{a_b}{c_d}');
    expect(after).toHaveLength(3);
    expect(after[0]).toBe(before[0]);
    expect(after[1]!.html).toContain('<mfrac>');
    expect(after.map(block => block.html).join('')).toBe(renderMarkdown(complete));
    const updated = renderStreamingMarkdownBlocks('math', complete + ' grows');
    expect(updated[1]).toBe(after[1]);
  });
});
