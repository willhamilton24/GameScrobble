import React from 'react';
import { createHash } from 'crypto';
import { renderToStaticMarkup } from 'react-dom/server';
import SampleProfile from './SampleProfile';
import { SAMPLE_PERIOD, SAMPLE_RECORDS, SAMPLE_PROFILE, SAMPLE_CSV_ROWS } from './sampleData';

it('derives all sample views from the same period records', () => {
  const p = SAMPLE_PROFILE;
  // The same fixture fingerprint must pass in both UTC and America/New_York.
  expect(createHash('sha256').update(JSON.stringify(SAMPLE_RECORDS)).digest('hex'))
    .toBe('2e4561af03699bb342ac71a485ef794dd033558bfd8c0f86c21cd86f6aa5970c');
  expect(SAMPLE_RECORDS.every(r => r.date >= SAMPLE_PERIOD.start && r.date <= SAMPLE_PERIOD.end)).toBe(true);
  expect(p.total_minutes).toBe(SAMPLE_RECORDS.reduce((sum, r) => sum + r.minutes, 0));
  expect(p.heatmap.reduce((sum, r) => sum + r.minutes, 0)).toBe(p.total_minutes);
  expect(p.total_games).toBe(new Set(SAMPLE_RECORDS.map(r => r.app_id)).size);
  p.top_games.forEach((game, i) => {
    const records = SAMPLE_RECORDS.filter(r => r.app_id === game.app_id);
    expect(game.minutes).toBe(records.reduce((sum, r) => sum + r.minutes, 0));
    expect(game.recent_minutes).toBe(records.filter(r => r.date >= '2026-12-18').reduce((sum, r) => sum + r.minutes, 0));
    if (i) expect(p.top_games[i - 1].minutes).toBeGreaterThanOrEqual(game.minutes);
    if (i < 5) expect(SAMPLE_CSV_ROWS[i + 1]).toBe([game.app_id, game.name, game.minutes, game.recent_minutes, '2026-12-31T23:59:59Z'].join(','));
  });
});

it.each(['2027-01-01T00:00:00Z', '2027-07-04T12:00:00Z', '2028-02-29T23:30:00Z', '2028-12-31T23:59:59Z'])(
  'keeps the complete fixed year when the clock is %s', now => {
    const OriginalDate = global.Date;
    const baseline = renderToStaticMarkup(<SampleProfile />);
    global.Date = class extends OriginalDate {
      constructor(...args) { super(...(args.length ? args : [now])); }
      static now() { return new OriginalDate(now).getTime(); }
    };
    try {
      const html = renderToStaticMarkup(<SampleProfile />);
      expect(html).toBe(baseline);
      expect(html.match(/<rect /g)).toHaveLength(365);
      expect(html).toContain('2026-01-01:');
      expect(html).toContain('2026-12-31:');
      expect(html).toContain('played in 2026');
      jest.resetModules();
      expect(require('./sampleData').SAMPLE_PROFILE).toEqual(SAMPLE_PROFILE);
    } finally {
      global.Date = OriginalDate;
    }
  }
);
