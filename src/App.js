import React, { useState, useEffect } from 'react';
import ProfilePage from './ProfilePage';
import Account from './Account';
import SampleProfile from './SampleProfile';
import './App.css';

const SESSION_KEY = 'gameroom.token';

// PremiumSuccess — /premium/success
// Polls GET /api/premium/status (up to _maxPolls times) to confirm server-side
// activation. The Stripe session_id in the URL is never used to grant access.
export function PremiumSuccess({ _maxPolls = 20, _pollIntervalMs = 3000 }) {
  const [phase, setPhase] = useState('loading'); // loading|pending|active|timed_out|error|unauthenticated
  const [userId, setUserId] = useState(null);
  const [errorMsg, setErrorMsg] = useState('');

  useEffect(() => {
    const token = localStorage.getItem(SESSION_KEY);
    if (!token) { setPhase('unauthenticated'); return; }
    let aborted = false;
    let timer = null;

    async function check(attempt) {
      if (aborted) return;
      try {
        const r = await fetch('/api/premium/status', {
          headers: { Authorization: `Bearer ${token}` },
          cache: 'no-store',
        });
        if (aborted) return;
        if (!r.ok) { setErrorMsg('Could not verify premium status.'); setPhase('error'); return; }
        const data = await r.json();
        if (aborted) return;
        if (data.premium) {
          setUserId(data.user_id || null);
          setPhase('active');
        } else if (attempt + 1 >= _maxPolls) {
          setPhase('timed_out');
        } else {
          setPhase('pending');
          timer = setTimeout(() => check(attempt + 1), _pollIntervalMs);
        }
      } catch (err) {
        if (aborted) return;
        setErrorMsg(err.message);
        setPhase('error');
      }
    }

    check(0);
    return () => { aborted = true; clearTimeout(timer); };
  }, [_maxPolls, _pollIntervalMs]);

  if (phase === 'unauthenticated') {
    return (
      <main>
        <div className="premium-screen">
          <h1>Sign in to continue</h1>
          <p>Sign in to your gameroom account to see your premium status.</p>
          <a href="/" className="compare-btn compare-btn--premium">Go home</a>
        </div>
      </main>
    );
  }

  if (phase === 'error') {
    return (
      <main>
        <div className="premium-screen">
          <h1>Something went wrong</h1>
          <p role="alert">{errorMsg || 'Unable to verify your premium status.'}</p>
          <a href="/" className="compare-btn compare-btn--free">Go home</a>
        </div>
      </main>
    );
  }

  if (phase === 'active') {
    return (
      <main>
        <div className="premium-screen">
          <h1>Premium activated</h1>
          <p>Your subscription is active. You now have access to deeper stats and CSV export.</p>
          <a href={`/u/export?user_id=${encodeURIComponent(userId)}`} className="compare-btn compare-btn--premium">
            Download game history (CSV)
          </a>
          <a href="/" className="compare-btn compare-btn--free">Go home</a>
        </div>
      </main>
    );
  }

  if (phase === 'timed_out') {
    return (
      <main>
        <div className="premium-screen">
          <h1>Activation is taking longer than expected</h1>
          <p>Your payment was received. Premium activation can take a few minutes — please check back shortly or refresh this page.</p>
          <a href="/" className="compare-btn compare-btn--free">Go home</a>
        </div>
      </main>
    );
  }

  // loading | pending
  return (
    <main>
      <div className="premium-screen">
        <h1>{phase === 'pending' ? 'Activating your subscription…' : 'Checking your account…'}</h1>
        <p role="status">{phase === 'pending' ? 'Waiting for payment confirmation…' : 'Please wait.'}</p>
      </div>
    </main>
  );
}

// PremiumCancel — /premium/cancel
export function PremiumCancel() {
  return (
    <main>
      <div className="premium-screen">
        <h1>Checkout cancelled</h1>
        <p>Your payment was cancelled. No charge was made.</p>
        <a href="/" className="compare-btn compare-btn--free">Go home</a>
      </div>
    </main>
  );
}

const COMPARE_ROWS = [
  { feature: 'Public profile & shareable URL',    free: true,  premium: true },
  { feature: 'Year-in-games activity heatmap',    free: true,  premium: true },
  { feature: 'Total hours, games & top game',     free: true,  premium: true },
  { feature: 'Per-game snapshot CSV export',           free: false, premium: true },
];

function FeatureComparison({ user, onPremium }) {
  return (
    <div className="feature-compare">
      <h2 className="compare-title" id="compare-title">Free vs Premium</h2>
      <table className="compare-table" aria-labelledby="compare-title">
        <thead><tr className="compare-header">
          <th scope="col">Feature</th>
          <th scope="col">Free</th>
          <th scope="col" className="compare-tier-premium">Premium</th>
        </tr></thead>
        <tbody>{COMPARE_ROWS.map(row => (
          <tr key={row.feature} className="compare-row">
            <th scope="row">{row.feature}</th>
            {[row.free, row.premium].map((included, i) => (
              <td key={i} className="compare-tier-col">
                <span aria-label={included ? 'Included' : 'Not included'}
                  className={included ? 'compare-check' : 'compare-dash'}>
                  <span aria-hidden="true">{included ? '✓' : '—'}</span>
                </span>
              </td>
            ))}
          </tr>
        ))}</tbody>
      </table>
      <div className="compare-ctas">
        <a href="#get-started" className="compare-btn compare-btn--free">
          {user ? 'Go to your account' : 'Get started free'}
        </a>
        <button type="button" className="compare-btn compare-btn--premium" onClick={onPremium}>
          {user ? 'Go to Premium checkout' : 'Sign up to continue to Premium checkout'}
        </button>
      </div>
    </div>
  );
}

function Home() {
  const [user, setUser] = useState(null);
  const [premiumRequest, setPremiumRequest] = useState(0);
  return (
    <main>
      <div className="hero-wrap">
        <h1>gameroom</h1>
        <p>Explore your Steam playtime with a public profile and daily heatmap.</p>
        <p className="tagline">See playtime changes observed since tracking began.</p>
        <div id="get-started">
          <Account onUserChange={setUser} premiumRequest={premiumRequest} />
        </div>
      </div>
      <SampleProfile />
      <FeatureComparison user={user} onPremium={() => setPremiumRequest(value => value + 1)} />
    </main>
  );
}

function App() {
  const path = window.location.pathname;
  if (path === '/premium/success') return <PremiumSuccess />;
  if (path === '/premium/cancel') return <PremiumCancel />;
  const match = path.match(/^\/u\/([^/]+)$/);
  if (match) return <ProfilePage userId={match[1]} />;
  return <Home />;
}

export default App;
