// Notes from agentmd and git often quote commands in backticks. This splits
// such text so the quoted parts can render as code.

export interface InlinePart {
  code: boolean;
  text: string;
}

export function inlineParts(text: string): InlinePart[] {
  const out: InlinePart[] = [];
  const re = /`([^`\n]+)`/g;
  let last = 0;
  for (let m = re.exec(text); m !== null; m = re.exec(text)) {
    if (m.index > last) out.push({ code: false, text: text.slice(last, m.index) });
    out.push({ code: true, text: m[1] });
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push({ code: false, text: text.slice(last) });
  return out;
}
