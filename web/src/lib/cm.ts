// CodeMirror pieces shared by the editor and the merge views.
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { markdownKeymap, markdownLanguage } from '@codemirror/lang-markdown';
import { HighlightStyle, Language, LanguageSupport, syntaxHighlighting } from '@codemirror/language';
import type { Diagnostic } from '@codemirror/lint';
import { highlightSelectionMatches, searchKeymap } from '@codemirror/search';
import { StateEffect, StateField, type Extension, type Text } from '@codemirror/state';
import {
  Decoration,
  drawSelection,
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  type DecorationSet,
} from '@codemirror/view';
import { tags } from '@lezer/highlight';
import { MarkdownParser, type MarkdownConfig } from '@lezer/markdown';
import { KIND_LABELS } from './findings';
import type { Finding } from './types';

export const editorTheme = EditorView.theme({
  '&': {
    color: 'var(--text)',
    backgroundColor: 'var(--editor-bg)',
    height: '100%',
    fontSize: '13px',
  },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.55' },
  '.cm-content': { caretColor: 'var(--text)', padding: '6px 0' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--text)' },
  '&.cm-focused': { outline: 'none' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'var(--selection-bg)',
  },
  '.cm-gutters': {
    backgroundColor: 'var(--editor-gutter-bg)',
    color: 'var(--text-3)',
    borderRight: '1px solid var(--border)',
  },
  '.cm-activeLine': { backgroundColor: 'var(--editor-active-line)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--editor-active-line)', color: 'var(--text-2)' },
  '.cm-lineNumbers .cm-gutterElement': { padding: '0 8px 0 10px', minWidth: '32px' },
  '.cm-agentmd-span': { backgroundColor: 'var(--span-bg)', boxShadow: 'inset 3px 0 0 var(--span-border)' },
  '.cm-searchMatch': { backgroundColor: 'var(--search-match)', outline: '1px solid var(--border-strong)' },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'var(--search-match-selected)' },
  '.cm-selectionMatch': { backgroundColor: 'var(--search-match)' },
  '.cm-panels': { backgroundColor: 'var(--surface-2)', color: 'var(--text)', borderColor: 'var(--border)' },
  '.cm-panels input, .cm-panels button': { fontFamily: 'var(--font-ui)', fontSize: '12px' },
  '.cm-tooltip': {
    backgroundColor: 'var(--surface)',
    color: 'var(--text)',
    border: '1px solid var(--border-strong)',
    borderRadius: '6px',
    boxShadow: 'var(--shadow)',
  },
  '.cm-diagnostic': { fontFamily: 'var(--font-ui)', fontSize: '12.5px', padding: '4px 8px' },
  '.cm-diagnostic-error': { borderLeft: '4px solid var(--problem)' },
  '.cm-diagnostic-warning': { borderLeft: '4px solid var(--hint)' },
  '.cm-lintRange-error': { backgroundImage: 'none', textDecoration: 'underline wavy var(--problem)' },
  '.cm-lintRange-warning': { backgroundImage: 'none', textDecoration: 'underline wavy var(--hint)' },
  '.cm-lint-marker-error': { content: 'none' },
  '.cm-gutter-lint': { width: '14px' },
  // Merge views: side a (before, or the disk version) in red, side b and
  // inserted lines of the unified view in green.
  '.cm-changedLine': { backgroundColor: 'var(--diff-add-bg) !important' },
  '.cm-changedText': { background: 'var(--diff-add-strong) !important' },
  '&.cm-merge-a .cm-changedLine': { backgroundColor: 'var(--diff-del-bg) !important' },
  '&.cm-merge-a .cm-changedText': { background: 'var(--diff-del-strong) !important' },
  '.cm-deletedChunk': { backgroundColor: 'var(--diff-del-bg) !important' },
  '.cm-deletedText, .cm-deletedChunk .cm-deletedText': { background: 'var(--diff-del-strong) !important' },
  '.cm-changeGutter': { width: '3px', paddingLeft: '1px' },
  '&.cm-merge-a .cm-changedLineGutter': { background: 'var(--problem)' },
  '&.cm-merge-b .cm-changedLineGutter, .cm-changedLineGutter': { background: 'var(--ok)' },
  '.cm-deletedLineGutter': { background: 'var(--problem)' },
  '.cm-collapsedLines': {
    background: 'var(--surface-2) !important',
    color: 'var(--text-2) !important',
    fontFamily: 'var(--font-ui)',
    fontSize: '12px',
  },
});

const markdownStyle = HighlightStyle.define([
  { tag: tags.heading1, fontWeight: '700', color: 'var(--md-heading)', fontSize: '1.08em' },
  { tag: [tags.heading2, tags.heading3, tags.heading4, tags.heading5, tags.heading6], fontWeight: '700', color: 'var(--md-heading)' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.strong, fontWeight: '700' },
  { tag: tags.strikethrough, textDecoration: 'line-through' },
  { tag: [tags.link, tags.url], color: 'var(--md-link)' },
  { tag: tags.monospace, color: 'var(--md-code)' },
  { tag: tags.quote, color: 'var(--text-2)', fontStyle: 'italic' },
  { tag: [tags.processingInstruction, tags.contentSeparator], color: 'var(--md-mark)' },
  { tag: [tags.comment, tags.meta], color: 'var(--text-3)' },
  { tag: [tags.keyword, tags.atom, tags.bool], color: 'var(--md-link)' },
  { tag: [tags.string], color: 'var(--md-code)' },
]);

/**
 * YAML frontmatter at the top of a skill, subagent or command. Plain markdown
 * would read its closing "---" as a heading underline and bold the last key.
 */
export const frontmatter: MarkdownConfig = {
  defineNodes: [{ name: 'Frontmatter', block: true, style: tags.meta }],
  parseBlock: [
    {
      name: 'Frontmatter',
      before: 'HorizontalRule',
      parse(cx, line) {
        if (cx.lineStart !== 0 || line.text.trimEnd() !== '---') return false;
        const from = cx.lineStart;
        while (cx.nextLine()) {
          const text = line.text.trimEnd();
          if (text === '---' || text === '...') {
            const to = cx.lineStart + line.text.length;
            cx.nextLine();
            cx.addElement(cx.elt('Frontmatter', from, to));
            return true;
          }
        }
        cx.addElement(cx.elt('Frontmatter', from, cx.prevLineEnd()));
        return true;
      },
    },
  ],
};

/** GitHub-flavoured markdown plus frontmatter, sharing lang-markdown's language data. */
const agentMarkdown: Language =
  markdownLanguage.parser instanceof MarkdownParser
    ? new Language(markdownLanguage.data, markdownLanguage.parser.configure([frontmatter]), [], 'markdown')
    : markdownLanguage;

/**
 * Markdown with the app's colours, for every editor and merge view. It skips
 * markdown()'s embedded HTML language, which would pull the HTML, CSS and
 * JavaScript parsers into the bundle for tags that agent files rarely contain.
 */
export function markdownExtensions(): Extension[] {
  return [new LanguageSupport(agentMarkdown), syntaxHighlighting(markdownStyle), editorTheme, EditorView.lineWrapping];
}

/**
 * Scrolls only the editor's own scroller. CodeMirror's default also scrolls
 * every scrollable ancestor, which would move the page when compare mode
 * centres the second file's span, or when the cursor nears an edge.
 */
const containedScroll = EditorView.scrollHandler.of((view, range, options) => {
  // Layout reads such as coordsAtPos are not allowed here; the height map and
  // documentTop are.
  const block = view.lineBlockAt(range.head);
  const top = view.documentTop + block.top;
  const bottom = top + block.height;
  const box = view.scrollDOM.getBoundingClientRect();
  const margin = options.yMargin;
  let delta = 0;
  if (options.y === 'center') delta = (top + bottom) / 2 - (box.top + box.height / 2);
  else if (options.y === 'start') delta = top - box.top - margin;
  else if (options.y === 'end') delta = bottom - box.bottom + margin;
  else if (top < box.top + margin) delta = top - box.top - margin;
  else if (bottom > box.bottom - margin) delta = bottom - box.bottom + margin;
  if (delta) view.scrollDOM.scrollTop += delta;
  return true;
});

/** The basics for an editable (or read-only but selectable) editor. */
export function editorBasics(): Extension[] {
  return [
    containedScroll,
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightSpecialChars(),
    history(),
    drawSelection(),
    highlightActiveLine(),
    highlightSelectionMatches(),
    keymap.of([...markdownKeymap, ...defaultKeymap, ...historyKeymap, ...searchKeymap, indentWithTab]),
    ...markdownExtensions(),
  ];
}

// ---- highlighted span ------------------------------------------------------

export interface LineSpan {
  start: number;
  end: number;
}

export const setSpan = StateEffect.define<LineSpan | null>();

const spanLine = Decoration.line({ attributes: { class: 'cm-agentmd-span' } });

export function clampSpan(doc: Text, span: LineSpan): LineSpan {
  const start = Math.min(Math.max(1, span.start), doc.lines);
  const end = Math.min(Math.max(start, span.end), doc.lines);
  return { start, end };
}

function spanDecorations(doc: Text, span: LineSpan | null): DecorationSet {
  if (!span) return Decoration.none;
  const { start, end } = clampSpan(doc, span);
  const ranges = [];
  for (let n = start; n <= end; n++) ranges.push(spanLine.range(doc.line(n).from));
  return Decoration.set(ranges);
}

export const spanField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    let next = deco.map(tr.changes);
    for (const e of tr.effects) if (e.is(setSpan)) next = spanDecorations(tr.state.doc, e.value);
    return next;
  },
  provide: (f) => EditorView.decorations.from(f),
});

/** Selects nothing but highlights the span and scrolls it to the middle. */
export function showSpan(view: EditorView, span: LineSpan | null) {
  const effects: StateEffect<unknown>[] = [setSpan.of(span)];
  if (span) {
    const { start } = clampSpan(view.state.doc, span);
    effects.push(EditorView.scrollIntoView(view.state.doc.line(start).from, { y: 'center' }));
  }
  view.dispatch({ effects });
}

// ---- findings as diagnostics -----------------------------------------------

/** One diagnostic per finding span in this file: problems are errors, hints warnings. */
export function findingDiagnostics(doc: Text, findings: Finding[], fileId: string): Diagnostic[] {
  const out: Diagnostic[] = [];
  if (doc.lines === 0) return out;
  for (const f of findings) {
    for (const s of f.spans) {
      if (s.fileId !== fileId) continue;
      const { start, end } = clampSpan(doc, { start: s.startLine, end: s.endLine });
      out.push({
        from: doc.line(start).from,
        to: doc.line(end).to,
        severity: f.severity === 'problem' ? 'error' : 'warning',
        message: `${KIND_LABELS[f.kind] ?? f.kind}: ${f.summary}`,
        source: f.source === 'analysis' ? 'agentmd analysis' : 'agentmd',
      });
    }
  }
  return out.sort((a, b) => a.from - b.from);
}
