// Fictional demo account – bundled in the client, no API calls, no real user data.

function generateHeatmap() {
  const entries = [];
  const start = new Date('2025-10-26');
  const end   = new Date('2026-10-25');
  let seed = 0x4a3f2b1c;
  function rand() {
    seed = (Math.imul(1664525, seed) + 1013904223) >>> 0;
    return seed / 4294967296;
  }
  const cur = new Date(start);
  while (cur <= end) {
    const dow = cur.getDay();
    const isWeekend = dow === 0 || dow === 6;
    if (rand() < (isWeekend ? 0.70 : 0.40)) {
      const base = isWeekend ? 90 : 45;
      const minutes = Math.floor(base + rand() * 240);
      entries.push({ date: cur.toISOString().slice(0, 10), minutes });
    }
    cur.setDate(cur.getDate() + 1);
  }
  return entries;
}

export const SAMPLE_PROFILE = {
  username: 'alex_void',
  year: 2026,
  total_minutes: 31500,
  total_games: 42,
  top_games: [
    { app_id: 1245620, name: 'Elden Ring',         minutes: 9240 },
    { app_id: 1086940, name: "Baldur's Gate 3",    minutes: 7020 },
    { app_id:  367520, name: 'Hollow Knight',      minutes: 3120 },
    { app_id:  504230, name: 'Celeste',            minutes: 1980 },
    { app_id: 1145360, name: 'Hades',              minutes: 1680 },
    { app_id:  524220, name: 'NieR:Automata',      minutes: 1560 },
  ],
  heatmap: generateHeatmap(),
};

export const SAMPLE_CSV_ROWS = [
  'app_id,name,playtime_total_minutes,playtime_recent_minutes,last_polled',
  '1245620,Elden Ring,9240,0,2026-10-25',
  "1086940,Baldur's Gate 3,7020,180,2026-10-25",
  '367520,Hollow Knight,3120,0,2026-10-18',
  '504230,Celeste,1980,0,2026-09-12',
  '1145360,Hades,1680,0,2026-08-30',
  '...',
];
