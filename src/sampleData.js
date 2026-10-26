// Bundled fictional records, independent of accounts and live ingestion.
// This sample starts with zero playtime on January 1; snapshots are at year end.
export const SAMPLE_PERIOD = {
  start: '2026-01-01',
  end: '2026-12-31',
  year: 2026,
  label: 'January 1–December 31, 2026 (UTC)',
};
const games = [
  { app_id: 1245620, name: 'Elden Ring' },
  { app_id: 1086940, name: "Baldur's Gate 3" },
  { app_id: 367520, name: 'Hollow Knight' },
  { app_id: 504230, name: 'Celeste' },
  { app_id: 1145360, name: 'Hades' },
  { app_id: 524220, name: 'NieR:Automata' },
];

function generateRecords() {
  const records = [];
  let seed = 0x4a3f2b1c;
  function rand() {
    seed = (Math.imul(1664525, seed) + 1013904223) >>> 0;
    return seed / 4294967296;
  }
  const cur = new Date(SAMPLE_PERIOD.start + 'T00:00:00Z');
  const end = new Date(SAMPLE_PERIOD.end + 'T00:00:00Z');
  while (cur <= end) {
    const weekend = cur.getUTCDay() === 0 || cur.getUTCDay() === 6;
    if (rand() < (weekend ? 0.70 : 0.40)) {
      records.push({
        ...games[Math.floor(rand() * games.length)],
        date: cur.toISOString().slice(0, 10),
        minutes: Math.floor((weekend ? 90 : 45) + rand() * 240),
      });
    }
    cur.setUTCDate(cur.getUTCDate() + 1);
  }
  return records;
}
export const SAMPLE_RECORDS = generateRecords();
const byDate = {};
const byGame = {};
SAMPLE_RECORDS.forEach(record => {
  byDate[record.date] = (byDate[record.date] || 0) + record.minutes;
  const game = byGame[record.app_id] || {
    app_id: record.app_id, name: record.name, minutes: 0, recent_minutes: 0,
    last_polled: SAMPLE_PERIOD.end + 'T23:59:59Z',
  };
  game.minutes += record.minutes;
  // Steam's recent-playtime snapshot covers the last two weeks.
  if (record.date >= '2026-12-18') game.recent_minutes += record.minutes;
  byGame[record.app_id] = game;
});
export const SAMPLE_PROFILE = {
  username: 'alex_void',
  year: SAMPLE_PERIOD.year,
  total_minutes: SAMPLE_RECORDS.reduce((sum, record) => sum + record.minutes, 0),
  total_games: Object.keys(byGame).length,
  top_games: Object.values(byGame).sort((a, b) => b.minutes - a.minutes),
  heatmap: Object.keys(byDate).sort().map(date => ({ date, minutes: byDate[date] })),
};
export const SAMPLE_CSV_ROWS = [
  'app_id,name,playtime_total_minutes,playtime_recent_minutes,last_polled',
  ...SAMPLE_PROFILE.top_games.slice(0, 5).map(game =>
    [game.app_id, game.name, game.minutes, game.recent_minutes, game.last_polled].join(',')
  ),
];
