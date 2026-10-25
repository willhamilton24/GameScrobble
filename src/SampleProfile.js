import React from 'react';
import { SAMPLE_PROFILE, SAMPLE_CSV_ROWS } from './sampleData';
import './ProfilePage.css';

function formatHours(minutes) {
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return m + 'm';
  if (m === 0) return h + 'h';
  return h + 'h ' + m + 'm';
}

function heatColor(minutes) {
  if (!minutes) return '#131328';
  if (minutes <= 30) return '#0f3460';
  if (minutes <= 90) return '#0a5c8a';
  if (minutes <= 180) return '#0096b4';
  return '#00d4ff';
}

function Heatmap({ data }) {
  const today = new Date();
  const start = new Date(today);
  start.setDate(start.getDate() - 363);
  start.setDate(start.getDate() - start.getDay());

  const byDate = {};
  (data || []).forEach(function(d) { byDate[d.date] = d.minutes; });

  const weeks = [];
  const cur = new Date(start);
  while (cur <= today) {
    const week = [];
    for (let d = 0; d < 7; d++) {
      const dateStr = cur.toISOString().slice(0, 10);
      week.push({ date: dateStr, minutes: byDate[dateStr] || 0 });
      cur.setDate(cur.getDate() + 1);
    }
    weeks.push(week);
  }

  const CELL = 11, GAP = 2;
  const w = weeks.length * (CELL + GAP);
  const h = 7 * (CELL + GAP);

  const months = [];
  let lastMonth = -1;
  weeks.forEach(function(week, wi) {
    const m = new Date(week[0].date).getUTCMonth();
    if (m !== lastMonth) {
      const label = new Date(week[0].date).toLocaleString('default', { month: 'short', timeZone: 'UTC' });
      months.push({ x: wi * (CELL + GAP), label });
      lastMonth = m;
    }
  });

  return (
    <div className="heatmap-wrap">
      <svg width={w} height={h + 18} style={{ overflow: 'visible', display: 'block' }}>
        {months.map(function(m, i) {
          return (
            <text key={i} x={m.x} y={10} fontSize={9} fill="#475569" fontFamily="inherit">
              {m.label}
            </text>
          );
        })}
        {weeks.map(function(week, wi) {
          return week.map(function(day, di) {
            return (
              <rect
                key={wi + '-' + di}
                x={wi * (CELL + GAP)} y={18 + di * (CELL + GAP)}
                width={CELL} height={CELL} rx={2}
                fill={heatColor(day.minutes)}
              >
                <title>{day.date}: {day.minutes > 0 ? formatHours(day.minutes) : 'No activity'}</title>
              </rect>
            );
          });
        })}
      </svg>
      <div className="heatmap-legend">
        <span>Less</span>
        {[0, 30, 90, 180, 240].map(function(m, i) {
          return <div key={i} className="legend-cell" style={{ background: heatColor(m + 1) }} />;
        })}
        <span>More</span>
      </div>
    </div>
  );
}

function SampleProfile() {
  const p = SAMPLE_PROFILE;
  const topGame = p.top_games[0];

  return (
    <div className="sample-demo">
      <div className="sample-demo-label">
        <span className="sample-badge">Sample Profile</span>
        <span className="sample-desc">Fictional activity — see what gameroom tracks for you</span>
      </div>

      <div className="sample-username">
        {p.username} &mdash; {p.year} in games
      </div>

      <section className="stats-bar">
        <div className="stat-card">
          <div className="stat-value">{formatHours(p.total_minutes)}</div>
          <div className="stat-label">played this year</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{p.total_games}</div>
          <div className="stat-label">games</div>
        </div>
        {topGame && (
          <div className="stat-card stat-card--wide">
            <div className="stat-value stat-value--sm">{topGame.name}</div>
            <div className="stat-label">most played &mdash; {formatHours(topGame.minutes)}</div>
          </div>
        )}
      </section>

      <section className="section">
        <div className="section-header">
          <h2 className="section-title">Activity</h2>
        </div>
        <div className="heatmap-card">
          <Heatmap data={p.heatmap} />
        </div>
      </section>

      <section className="section">
        <div className="section-header">
          <h2 className="section-title">Top Games</h2>
        </div>
        <div className="games-grid">
          {p.top_games.map(function(game) {
            return (
              <div key={game.app_id + '-' + game.name} className="game-card">
                <img
                  src={'https://cdn.akamai.steamstatic.com/steam/apps/' + game.app_id + '/header.jpg'}
                  alt={game.name}
                  className="game-cover"
                  onError={function(e) { e.target.style.display = 'none'; }}
                />
                <div className="game-info">
                  <div className="game-name" title={game.name}>{game.name}</div>
                  <div className="game-time">{formatHours(game.minutes)}</div>
                </div>
              </div>
            );
          })}
        </div>
      </section>

      <div className="csv-preview-block">
        <div className="section-header">
          <h2 className="section-title">Premium &mdash; CSV export preview</h2>
        </div>
        <div className="csv-preview">
          <pre className="csv-code">{SAMPLE_CSV_ROWS.join('\n')}</pre>
          <p className="csv-note">
            Premium subscribers download their full history as a spreadsheet — every game, playtime, and sync date.
          </p>
        </div>
      </div>
    </div>
  );
}

export default SampleProfile;
