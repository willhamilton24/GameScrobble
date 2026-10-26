import React, { useState } from 'react';
import ProfilePage from './ProfilePage';
import Account from './Account';
import SampleProfile from './SampleProfile';
import './App.css';

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
  const match = window.location.pathname.match(/^\/u\/([^/]+)$/);
  if (match) {
    return <ProfilePage userId={match[1]} />;
  }
  return <Home />;
}

export default App;
