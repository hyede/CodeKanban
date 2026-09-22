import type { JSONContent } from '@tiptap/core';

import type { CodexSkillSummary } from '@/types/models';
import { filterCodexSkills } from '@/components/web-session/webSessionCodexSkills';

export interface WebSessionComposerSelection {
  start: number;
  end: number;
}

export interface WebSessionComposerEditorExposed {
  focus: () => void;
  getSelectionRange: () => WebSessionComposerSelection;
  setSelectionRange: (start: number, end?: number) => void;
}

export interface WebSessionComposerCompletionOption {
  key: string;
  label: string;
  detail: string;
  apply: string;
}

export interface WebSessionComposerCompletionResult {
  from: number;
  to: number;
  options: WebSessionComposerCompletionOption[];
}

export type WebSessionComposerDocument =
  | JSONContent
  | {
      toJSON: () => JSONContent;
    };

export type WebSessionComposerHighlightKind = 'skill' | 'unknown-skill' | 'goal' | 'compact';

export interface WebSessionComposerHighlightRange {
  from: number;
  to: number;
  kind: WebSessionComposerHighlightKind;
}

export type WebSessionComposerKeyAction =
  | 'none'
  | 'completion-next'
  | 'completion-previous'
  | 'completion-close'
  | 'completion-apply'
  | 'submit'
  | 'hard-break';

export interface WebSessionComposerKeyInput {
  key: string;
  altKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  shiftKey?: boolean;
  isComposing?: boolean;
  keyCode?: number;
  completionOpen?: boolean;
}

export type WebSessionComposerCompositionAction =
  | { type: 'none' }
  | { type: 'apply-external'; value: string }
  | { type: 'emit-local'; value: string };

export function resolveWebSessionComposerCompositionEnd(input: {
  startValue: string;
  localValue: string;
  modelValue: string;
  pendingExternalValue: string | null;
}): WebSessionComposerCompositionAction {
  if (input.localValue !== input.startValue) {
    return input.localValue === input.modelValue
      ? { type: 'none' }
      : { type: 'emit-local', value: input.localValue };
  }

  if (input.pendingExternalValue != null && input.modelValue !== input.localValue) {
    return { type: 'apply-external', value: input.modelValue };
  }

  return input.localValue === input.modelValue
    ? { type: 'none' }
    : { type: 'emit-local', value: input.localValue };
}

export function composerTextToJSON(value: string): JSONContent {
  const text = normalizeComposerLineBreaks(String(value ?? ''));
  const content: JSONContent[] = [];
  let segmentStart = 0;

  for (let index = 0; index < text.length; index += 1) {
    if (text[index] !== '\n') {
      continue;
    }

    if (index > segmentStart) {
      content.push({ type: 'text', text: text.slice(segmentStart, index) });
    }
    content.push({ type: 'hardBreak' });
    segmentStart = index + 1;
  }

  if (segmentStart < text.length) {
    content.push({ type: 'text', text: text.slice(segmentStart) });
  }

  return {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        ...(content.length > 0 ? { content } : {}),
      },
    ],
  };
}

function normalizeComposerLineBreaks(value: string) {
  return value.replace(/\r\n?/g, '\n');
}

function readComposerDocument(value: WebSessionComposerDocument): JSONContent {
  if (typeof value === 'object' && value !== null && 'toJSON' in value) {
    return value.toJSON();
  }
  return value;
}

interface ComposerBoundary {
  offset: number;
  position: number;
}

interface ComposerParagraphLayout {
  contentStart: number;
  contentEnd: number;
  nodeStart: number;
  nodeEnd: number;
  startOffset: number;
  endOffset: number;
  boundaries: ComposerBoundary[];
}

interface ComposerDocumentLayout {
  text: string;
  boundaries: number[];
  paragraphs: ComposerParagraphLayout[];
}

function appendNormalizedText(
  text: string,
  startPosition: number,
  output: string[],
  boundaries: number[],
  paragraphBoundaries: ComposerBoundary[]
) {
  let rawIndex = 0;
  while (rawIndex < text.length) {
    const character = text[rawIndex];
    const rawLength = character === '\r' && text[rawIndex + 1] === '\n' ? 2 : 1;
    output.push(character === '\r' ? '\n' : character);
    rawIndex += rawLength;
    const position = startPosition + rawIndex;
    boundaries.push(position);
    paragraphBoundaries.push({ offset: output.length, position });
  }
  return startPosition + rawIndex;
}

function buildComposerDocumentLayout(value: WebSessionComposerDocument): ComposerDocumentLayout {
  const document = readComposerDocument(value);
  const paragraphs =
    document.type === 'paragraph'
      ? [document]
      : (document.content ?? []).filter(node => node.type === 'paragraph');
  const normalizedParagraphs = paragraphs.length > 0 ? paragraphs : [{ type: 'paragraph' }];
  const output: string[] = [];
  const boundaries: number[] = [];
  const paragraphLayouts: ComposerParagraphLayout[] = [];
  let contentStart = 1;

  normalizedParagraphs.forEach((paragraph, paragraphIndex) => {
    const startOffset = output.length;
    const paragraphBoundaries: ComposerBoundary[] = [
      { offset: startOffset, position: contentStart },
    ];
    if (boundaries.length === 0) {
      boundaries.push(contentStart);
    }

    let position = contentStart;
    for (const node of paragraph.content ?? []) {
      if (node.type === 'text') {
        const next = appendNormalizedText(
          String(node.text ?? ''),
          position,
          output,
          boundaries,
          paragraphBoundaries
        );
        position = next;
        continue;
      }
      if (node.type === 'hardBreak') {
        output.push('\n');
        position += 1;
        boundaries.push(position);
        paragraphBoundaries.push({ offset: output.length, position });
      }
    }

    const contentEnd = position;
    const nodeStart = contentStart - 1;
    const nodeEnd = contentEnd + 1;
    const endOffset = output.length;
    paragraphLayouts.push({
      contentStart,
      contentEnd,
      nodeStart,
      nodeEnd,
      startOffset,
      endOffset,
      boundaries: paragraphBoundaries,
    });

    if (paragraphIndex < normalizedParagraphs.length - 1) {
      output.push('\n');
      const nextContentStart = contentEnd + 2;
      boundaries.push(nextContentStart);
      contentStart = nextContentStart;
    }
  });

  return {
    text: output.join(''),
    boundaries,
    paragraphs: paragraphLayouts,
  };
}

export function composerJSONToText(value: WebSessionComposerDocument): string {
  return buildComposerDocumentLayout(value).text;
}

function clampInteger(value: number, minimum: number, maximum: number) {
  if (!Number.isFinite(value)) {
    return minimum;
  }
  return Math.max(minimum, Math.min(Math.trunc(value), maximum));
}

export function composerOffsetToPosition(offset: number, document: WebSessionComposerDocument) {
  const layout = buildComposerDocumentLayout(document);
  const safeOffset = clampInteger(offset, 0, layout.text.length);
  const lastParagraph = layout.paragraphs[layout.paragraphs.length - 1];
  // A paragraph separator maps to the end of the preceding paragraph; the offset after it maps
  // to the next paragraph's content start.
  return layout.boundaries[safeOffset] ?? lastParagraph?.contentEnd ?? 1;
}

function positionToParagraphOffset(paragraph: ComposerParagraphLayout, position: number) {
  const boundaries = paragraph.boundaries;
  if (position <= boundaries[0]!.position) {
    return boundaries[0]!.offset;
  }
  for (let index = 1; index < boundaries.length; index += 1) {
    const boundary = boundaries[index]!;
    if (position === boundary.position) {
      return boundary.offset;
    }
    if (position < boundary.position) {
      return boundary.offset;
    }
  }
  return boundaries[boundaries.length - 1]!.offset;
}

export function composerPositionToOffset(position: number, document: WebSessionComposerDocument) {
  const layout = buildComposerDocumentLayout(document);
  const lastParagraph = layout.paragraphs[layout.paragraphs.length - 1]!;
  const maximumPosition = lastParagraph.nodeEnd;
  const safePosition = clampInteger(position, 0, maximumPosition);

  for (const paragraph of layout.paragraphs) {
    if (safePosition < paragraph.contentStart) {
      return paragraph.startOffset > 0 ? paragraph.startOffset - 1 : 0;
    }
    if (safePosition <= paragraph.contentEnd) {
      return positionToParagraphOffset(paragraph, safePosition);
    }
    if (safePosition <= paragraph.nodeEnd) {
      return paragraph.endOffset;
    }
  }

  return layout.text.length;
}

export function buildWebSessionComposerHighlights(
  text: string,
  skills: readonly Pick<CodexSkillSummary, 'name'>[],
  goalEnabled = true
) {
  const knownSkills = new Set(skills.map(skill => skill.name.trim().toLowerCase()).filter(Boolean));
  const ranges: WebSessionComposerHighlightRange[] = [];

  for (const match of String(text ?? '').matchAll(/\$[a-z0-9][a-z0-9._-]*/gi)) {
    const token = match[0];
    const from = match.index;
    ranges.push({
      from,
      to: from + token.length,
      kind: knownSkills.has(token.slice(1).toLowerCase()) ? 'skill' : 'unknown-skill',
    });
  }

  if (goalEnabled) {
    for (const match of String(text ?? '').matchAll(/^\/goal\b/gm)) {
      const from = match.index;
      ranges.push({
        from,
        to: from + match[0].length,
        kind: 'goal',
      });
    }
  }

  for (const match of String(text ?? '').matchAll(/^\/compact(?:\s|$)/gm)) {
    const from = match.index;
    ranges.push({
      from,
      to: from + '/compact'.length,
      kind: 'compact',
    });
  }

  return ranges.sort((left, right) => left.from - right.from || left.to - right.to);
}

export function buildWebSessionComposerCompletions(
  text: string,
  cursor: number,
  skills: CodexSkillSummary[],
  goalEnabled = true
): WebSessionComposerCompletionResult {
  const normalizedText = String(text ?? '');
  const safeCursor = Math.max(0, Math.min(cursor, normalizedText.length));
  const beforeCursor = normalizedText.slice(0, safeCursor);
  const slashMatch = beforeCursor.match(/^\/[a-z-]*$/i);
  if (slashMatch) {
    const query = slashMatch[0].slice(1).toLowerCase();
    const slashOptions: WebSessionComposerCompletionOption[] = [
      ...(goalEnabled
        ? [{ key: 'slash:goal', label: '/goal', detail: 'Persistent goal', apply: '/goal ' }]
        : []),
      {
        key: 'slash:compact',
        label: '/compact',
        detail: 'Summarize conversation',
        apply: '/compact ',
      },
    ];
    const options = slashOptions.filter(option => option.label.slice(1).startsWith(query));
    return {
      from: 0,
      to: safeCursor,
      options,
    };
  }

  const skillMatch = beforeCursor.match(/\$[a-z0-9._-]*$/i);
  if (skillMatch) {
    const query = skillMatch[0].slice(1);
    const options = filterCodexSkills(skills, query)
      .slice(0, 12)
      .map(skill => {
        const details = [
          skill.displayName && skill.displayName !== skill.name ? skill.displayName : '',
          skill.source,
        ]
          .map(value => String(value || '').trim())
          .filter(Boolean);
        return {
          key: `skill:${skill.name}`,
          label: `$${skill.name}`,
          detail: details.join(' · '),
          apply: `$${skill.name}`,
        };
      });
    return {
      from: safeCursor - skillMatch[0].length,
      to: safeCursor,
      options,
    };
  }

  return {
    from: safeCursor,
    to: safeCursor,
    options: [],
  };
}

export function resolveWebSessionComposerKeyAction(
  input: WebSessionComposerKeyInput
): WebSessionComposerKeyAction {
  if (input.isComposing || input.keyCode === 229) {
    return 'none';
  }

  if (input.completionOpen) {
    if (input.key === 'ArrowDown') {
      return 'completion-next';
    }
    if (input.key === 'ArrowUp') {
      return 'completion-previous';
    }
    if (input.key === 'Escape') {
      return 'completion-close';
    }
  }

  if (input.key !== 'Enter') {
    return 'none';
  }

  const hasModifier = input.altKey || input.ctrlKey || input.metaKey || input.shiftKey;
  if (hasModifier) {
    return 'hard-break';
  }

  return input.completionOpen ? 'completion-apply' : 'submit';
}
