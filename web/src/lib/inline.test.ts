import { describe, expect, it } from 'vitest';
import { inlineParts } from './inline';

describe('inlineParts', () => {
  it('splits backtick spans out as code', () => {
    expect(inlineParts('Refreshed by `skills update`; edits here are replaced')).toEqual([
      { code: false, text: 'Refreshed by ' },
      { code: true, text: 'skills update' },
      { code: false, text: '; edits here are replaced' },
    ]);
  });

  it('handles code at either end and several spans', () => {
    expect(inlineParts('`a` and `b`')).toEqual([
      { code: true, text: 'a' },
      { code: false, text: ' and ' },
      { code: true, text: 'b' },
    ]);
  });

  it('leaves unmatched backticks and empty text alone', () => {
    expect(inlineParts('a ` b')).toEqual([{ code: false, text: 'a ` b' }]);
    expect(inlineParts('``')).toEqual([{ code: false, text: '``' }]);
    expect(inlineParts('')).toEqual([]);
  });
});
