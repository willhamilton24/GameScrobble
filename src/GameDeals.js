import React, { useEffect, useState } from 'react';
import './GameDeals.css';

const platforms = [['pc_deals', 'PC'], ['xbox_deals', 'Xbox'],
  ['playstation_deals', 'PlayStation'], ['switch_deals', 'Switch']];

function safeURL(value) {
  if (typeof value !== 'string') return null;
  try {
    const url = new URL(value);
    return ['https:', 'http:'].includes(url.protocol) ? value : null;
  } catch (err) {
    return null;
  }
}

export default function GameDeals({ title }) {
  const [state, setState] = useState({ status: 'loading' });
  useEffect(() => {
    let active = true;
    setState({ status: 'loading' });
    async function load() {
      try {
        const response = await fetch(`/game/${encodeURIComponent(title)}/deals`);
        if (response.status === 404) {
          if (active) setState({ status: 'missing' });
          return;
        }
        if (!response.ok) throw new Error('Deal lookup failed');
        const body = await response.json();
        const offers = [];
        platforms.forEach(([key, platform]) => {
          (Array.isArray(body[key]) ? body[key] : []).forEach(offer => {
            if (offer && safeURL(offer.deal_url)) {
              offers.push({ url: offer.deal_url, vendor: offer.vendor || 'Store', platform });
            }
          });
        });
        if (active) setState({ status: 'ready', offers, more: safeURL(body.find_more_deals) });
      } catch (err) {
        if (active) setState({ status: 'error' });
      }
    }
    load();
    return () => { active = false; };
  }, [title]);

  return <div className="game-deals" aria-label={`Deals for ${title}`}>
    <strong>Deals · affiliate links</strong>
    <small>We may earn a commission from purchases.</small>
    {state.status === 'loading' && <span>Loading deals…</span>}
    {state.status === 'missing' && <span>No deals found for this game.</span>}
    {state.status === 'error' && <span>Deals are temporarily unavailable.</span>}
    {state.status === 'ready' && <React.Fragment>
      {state.offers.length === 0 && <span>No current offers.</span>}
      {state.offers.map((offer, index) => <a key={index} href={offer.url} target="_blank" rel="sponsored noopener noreferrer">
        {offer.vendor} — {offer.platform} (affiliate link)
      </a>)}
      {state.more && <a href={state.more} target="_blank" rel="sponsored noopener noreferrer">Find more deals (affiliate link)</a>}
    </React.Fragment>}
  </div>;
}
