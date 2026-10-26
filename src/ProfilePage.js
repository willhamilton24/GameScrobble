import React, { useEffect, useState } from 'react';
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
  start.setDate(start.getDate() - start.getDay()); // align to Sunday

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

  const CELL = 11;
  const GAP = 2;
  const w = weeks.length * (CELL + GAP);
  const h = 7 * (CELL + GAP);

  // Month labels
  const months = [];
  let lastMonth = -1;
  weeks.forEach(function(week, wi) {
    const m = new Date(week[0].date).getUTCMonth();
    if (m !== lastMonth) {
      const label = new Date(week[0].date).toLocaleString('default', { month: 'short', timeZone: 'UTC' });
      months.push({ x: wi * (CELL + GAP), label: label });
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
                x={wi * (CELL + GAP)}
                y={18 + di * (CELL + GAP)}
                width={CELL}
                height={CELL}
                rx={2}
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

function GameCard({ game }) {
  const cover = 'https://cdn.akamai.steamstatic.com/steam/apps/' + game.app_id + '/header.jpg';
  return (
    <div className="game-card">
      <img
        src={cover}
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
}

function ProfilePage({ userId }) {
  const [profile, setProfile] = useState(null);
  const [loading, setLoading] = useState(true);
  const [errorType, setErrorType] = useState(null); // 'not-found' | 'error'
  const [fetchKey, setFetchKey] = useState(0);
  const [copied, setCopied] = useState(false);
  const [showUrl, setShowUrl] = useState(false);

  useEffect(function() {
    setLoading(true);
    setErrorType(null);
    setProfile(null);
    fetch('/u/' + userId + '/profile')
      .then(function(r) {
        if (r.status === 404) {
          var err = new Error('not-found');
          err.type = 'not-found';
          throw err;
        }
        if (!r.ok) {
          var err2 = new Error('error');
          err2.type = 'error';
          throw err2;
        }
        return r.json();
      })
      .then(function(data) {
        setProfile(data);
        setLoading(false);
      })
      .catch(function(err) {
        setErrorType(err.type || 'error');
        setLoading(false);
      });
  }, [userId, fetchKey]);

  function handleRetry() {
    setFetchKey(function(k) { return k + 1; });
  }

  function handleShare() {
    var url = window.location.href;
    if (navigator.clipboard) {
      navigator.clipboard.writeText(url).then(function() {
        setCopied(true);
        setTimeout(function() { setCopied(false); }, 2000);
      }).catch(function() {
        setShowUrl(true);
      });
    } else {
      setShowUrl(true);
    }
  }

  if (loading) {
    return <div className="profile-state" role="status">Loading...</div>;
  }
  if (errorType === 'not-found') {
    return (
      <div className="profile-state">
        <h2>No profile found</h2>
        <p>No gaming data for &ldquo;{userId}&rdquo; yet.</p>
        <a href="/" className="cta-btn" style={{ marginTop: '1.5rem', display: 'inline-block' }}>
          Get gameroom
        </a>
      </div>
    );
  }
  if (errorType === 'error') {
    return (
      <div className="profile-state" role="alert">
        <h2>Something went wrong</h2>
        <p>The profile couldn&rsquo;t be loaded right now.</p>
        <button className="cta-btn" style={{ marginTop: '1.5rem', border: 'none', cursor: 'pointer' }} onClick={handleRetry}>
          Retry
        </button>
      </div>
    );
  }

  const topGame = profile.top_games && profile.top_games[0];

  return (
    <div className="profile-page">
      <header className="profile-header">
        <div className="header-left">
          <div className="profile-id">{userId}</div>
          <span className="year-badge">{profile.year} in games</span>
        </div>
        <div className="share-wrap">
          <button className="share-btn" onClick={handleShare}>
            {copied ? '✓ Copied' : 'Share'}
          </button>
          {showUrl && (
            <input
              className="share-url"
              readOnly
              value={window.location.href}
              onFocus={function(e) { e.target.select(); }}
              aria-label="Profile URL"
              autoFocus
            />
          )}
        </div>
      </header>

      <section className="stats-bar">
        <div className="stat-card">
          <div className="stat-value">{formatHours(profile.total_minutes)}</div>
          <div className="stat-label">played this year</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{profile.total_games}</div>
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
          <Heatmap data={profile.heatmap} />
        </div>
      </section>

      {profile.top_games && profile.top_games.length > 0 && (
        <section className="section">
          <div className="section-header">
            <h2 className="section-title">Top Games</h2>
          </div>
          <div className="games-grid">
            {profile.top_games.map(function(game) {
              return <GameCard key={game.app_id} game={game} />;
            })}
          </div>
        </section>
      )}

      {profile.total_games === 0 && (
        <div className="empty-state">
          <p>No gaming activity recorded yet.</p>
          <p>Link your Steam account to start tracking.</p>
        </div>
      )}

      <footer className="profile-footer">
        <div className="cta-block">
          <p className="cta-copy">Track your own gaming history across Steam, Xbox, PSN, and Epic.</p>
          <a href="/" className="cta-btn">Get gameroom</a>
        </div>
        <p className="footer-brand">gameroom</p>
      </footer>
    </div>
  );
}

export default ProfilePage;
