import React, { useEffect, useState } from 'react';
import './PremiumStats.css';

export function formatPlaytime(minutes) {
  return `${Math.floor(minutes / 60).toLocaleString()}h ${minutes % 60}m`;
}

export default function PremiumStats({ userId, token, onUpgrade, busy }) {
  const [state, setState] = useState({ status: 'loading', games: [] });
  const [attempt, setAttempt] = useState(0);
  const [sort, setSort] = useState({ key: 'playtime_total', descending: true });

  useEffect(() => {
    let active = true;
    setState({ status: 'loading', games: [] });
    async function load() {
      try {
        const response = await fetch('/u/stats', {
          headers: { Authorization: `Bearer ${token}` }, cache: 'no-store'
        });
        if (response.status === 402) {
          if (active) setState({ status: 'upgrade', games: [] });
          return;
        }
        if (!response.ok) throw new Error('Unable to load stats');
        const body = await response.json();
        if (!Array.isArray(body.stats) || body.stats.some(game =>
          !Number.isInteger(game.app_id) || typeof game.name !== 'string' ||
          !Number.isInteger(game.playtime_total) || game.playtime_total < 0 ||
          !Number.isInteger(game.playtime_recent) || game.playtime_recent < 0 ||
          !Number.isFinite(Date.parse(game.last_polled)))) {
          throw new Error('Invalid stats response');
        }
        if (active) setState({ status: 'ready', games: body.stats });
      } catch (err) {
        if (active) setState({ status: 'error', games: [] });
      }
    }
    load();
    return () => { active = false; };
  }, [userId, token, attempt]);

  function changeSort(key) {
    setSort(previous => ({ key, descending: previous.key === key ? !previous.descending : key !== 'name' }));
  }

  const games = state.games.slice().sort((a, b) => {
    const result = sort.key === 'name' ? a.name.localeCompare(b.name) :
      sort.key === 'last_polled' ? Date.parse(a.last_polled) - Date.parse(b.last_polled) :
        a[sort.key] - b[sort.key];
    return (sort.descending ? -result : result) || a.app_id - b.app_id;
  });
  const columns = [['name', 'Game'], ['playtime_total', 'All-time playtime'],
    ['playtime_recent', 'Recent playtime'], ['last_polled', 'Last synced']];

  return (
    <section className="premium-stats" aria-labelledby="premium-stats-title" aria-busy={state.status === 'loading'}>
      <h3 id="premium-stats-title">Premium stats</h3>
      {state.status === 'loading' && <p role="status">Loading your stats…</p>}
      {state.status === 'upgrade' && <div>
        <p>Unlock all-time playtime and sortable game stats with Premium.</p>
        <button type="button" onClick={onUpgrade} disabled={busy}>Upgrade to Premium</button>
        <button type="button" onClick={() => setAttempt(attempt + 1)}>Check access again</button>
      </div>}
      {state.status === 'error' && <div>
        <p role="alert">We couldn’t load your stats. Please try again.</p>
        <button type="button" onClick={() => setAttempt(attempt + 1)}>Retry stats</button>
      </div>}
      {state.status === 'ready' && <div>
        <p className="stats-total">All-time playtime: <strong>{formatPlaytime(games.reduce((total, game) => total + game.playtime_total, 0))}</strong></p>
        <p>Across {games.length} tracked {games.length === 1 ? 'game' : 'games'}. Totals reflect the games synced to gameroom, not your entire library. Recent playtime is Steam’s reported two-week total at the last sync.</p>
        {games.length === 0 ? <p role="status">No game stats yet. Once a linked Steam account syncs played games, they’ll appear here.</p> :
          <div className="stats-table-scroll" role="region" aria-label="Game stats table" tabIndex={0}>
            <table>
              <caption>Your games — select a column to sort</caption>
              <thead><tr>{columns.map(([key, label]) => <th key={key} scope="col"
                aria-sort={sort.key === key ? (sort.descending ? 'descending' : 'ascending') : 'none'}>
                <button type="button" onClick={() => changeSort(key)}>{label}{sort.key === key ? (sort.descending ? ' ↓' : ' ↑') : ''}</button>
              </th>)}</tr></thead>
              <tbody>{games.map(game => <tr key={game.app_id}>
                <th scope="row">{game.name || `Game ${game.app_id}`}</th>
                <td>{formatPlaytime(game.playtime_total)}</td>
                <td>{formatPlaytime(game.playtime_recent)}</td>
                <td><time dateTime={game.last_polled}>{new Date(game.last_polled).toLocaleString()}</time></td>
              </tr>)}</tbody>
            </table>
          </div>}
      </div>}
    </section>
  );
}
