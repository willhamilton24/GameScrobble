import React, { useEffect, useState } from 'react';
import './PremiumStats.css';
import GameDeals from './GameDeals';

export function formatPlaytime(minutes) {
  return `${Math.floor(minutes / 60).toLocaleString()}h ${minutes % 60}m`;
}

export default function PremiumStats({ userId, token, onUpgrade, busy }) {
  const [state, setState] = useState({ status: 'loading', games: [] });
  const [attempt, setAttempt] = useState(0);
  const [sort, setSort] = useState({ key: 'playtime_total', descending: true });
  const [filter, setFilter] = useState({ query: '', activeOnly: false });

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

  const topGameIds = state.games.slice().sort((a, b) =>
    b.playtime_total - a.playtime_total || a.app_id - b.app_id
  ).slice(0, 5).map(game => game.app_id);

  const hasFilters = filter.query !== '' || filter.activeOnly;

  const filtered = state.games.filter(game =>
    (!filter.query || game.name.toLowerCase().includes(filter.query.toLowerCase())) &&
    (!filter.activeOnly || game.playtime_recent > 0)
  );

  const games = filtered.slice().sort((a, b) => {
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
        <p role="alert">We couldn't load your stats. Please try again.</p>
        <button type="button" onClick={() => setAttempt(attempt + 1)}>Retry stats</button>
      </div>}
      {state.status === 'ready' && <div>
        <p className="stats-total">All-time playtime: <strong>{formatPlaytime(state.games.reduce((total, game) => total + game.playtime_total, 0))}</strong></p>
        <p>Across {state.games.length} tracked {state.games.length === 1 ? 'game' : 'games'}. Totals reflect the games synced to gameroom, not your entire library. Recent playtime is Steam's reported two-week total at the last sync.</p>
        {state.games.length > 0 && <div className="stats-filters">
          <input
            id="stats-search"
            type="search"
            value={filter.query}
            onChange={e => { const query = e.target.value; setFilter(f => ({ ...f, query })); }}
            placeholder="Search games…"
            aria-label="Search games"
          />
          <label className="stats-filter-active">
            <input
              type="checkbox"
              checked={filter.activeOnly}
              onChange={e => { const activeOnly = e.target.checked; setFilter(f => ({ ...f, activeOnly })); }}
            />
            {' '}Active in the last two weeks
          </label>
          {hasFilters && <button type="button" onClick={() => setFilter({ query: '', activeOnly: false })}>Clear filters</button>}
        </div>}
        {state.games.length === 0
          ? <p role="status">No game stats yet. Once a linked Steam account syncs played games, they'll appear here.</p>
          : games.length === 0
            ? <p role="status">No games match your filters.</p>
            : <div className="stats-table-scroll" role="region" aria-label="Game stats table" tabIndex={0}>
                <table>
                  <caption>Your games — select a column to sort. Affiliate deals are shown for your five most-played games.</caption>
                  <thead><tr>{columns.map(([key, label]) => <th key={key} scope="col"
                    aria-sort={sort.key === key ? (sort.descending ? 'descending' : 'ascending') : 'none'}>
                    <button type="button" onClick={() => changeSort(key)}>{label}{sort.key === key ? (sort.descending ? ' ↓' : ' ↑') : ''}</button>
                  </th>)}<th scope="col">Deals (affiliate links)</th></tr></thead>
                  <tbody>{games.map(game => <tr key={game.app_id}>
                    <th scope="row">{game.name || `Game ${game.app_id}`}</th>
                    <td>{formatPlaytime(game.playtime_total)}</td>
                    <td>{formatPlaytime(game.playtime_recent)}</td>
                    <td><time dateTime={game.last_polled}>{new Date(game.last_polled).toLocaleString()}</time></td>
                    <td>{topGameIds.includes(game.app_id) ? <GameDeals title={game.name} /> : '—'}</td>
                  </tr>)}</tbody>
                </table>
              </div>}
      </div>}
    </section>
  );
}
