import React from 'react';
import ProfilePage from './ProfilePage';
import Account from './Account';
import SampleProfile from './SampleProfile';
import './App.css';

const COMPARE_ROWS = [
  { feature: 'Public profile & shareable URL',    free: true,  premium: true },
  { feature: 'Year-in-games activity heatmap',    free: true,  premium: true },
  { feature: 'Total hours, games & top game',     free: true,  premium: true },
  { feature: 'Full history CSV export',           free: false, premium: true },
  { feature: 'All-time playtime totals',          free: false, premium: true },
  { feature: 'Sortable game library',             free: false, premium: true },
  { feature: 'Priority sync',                     free: false, premium: true },
];

function FeatureComparison() {
  function scrollToSignup(e) {
    e.preventDefault();
    var el = document.getElementById('get-started');
    if (el) el.scrollIntoView({ behavior: 'smooth' });
  }

  return (
    <div className="feature-compare">
      <h2 className="compare-title">Free vs Premium</h2>
      <div className="compare-table">
        <div className="compare-header">
          <div className="compare-feature-col"></div>
          <div className="compare-tier-col">Free</div>
          <div className="compare-tier-col compare-tier-premium">Premium</div>
        </div>
        {COMPARE_ROWS.map(function(row, i) {
          return (
            <div key={i} className="compare-row">
              <div className="compare-feature-col">{row.feature}</div>
              <div className="compare-tier-col">
                {row.free
                  ? <span className="compare-check">&#10003;</span>
                  : <span className="compare-dash">&mdash;</span>}
              </div>
              <div className="compare-tier-col">
                {row.premium
                  ? <span className="compare-check compare-check--premium">&#10003;</span>
                  : <span className="compare-dash">&mdash;</span>}
              </div>
            </div>
          );
        })}
      </div>
      <div className="compare-ctas">
        <a href="#get-started" className="compare-btn compare-btn--free" onClick={scrollToSignup}>
          Get started free
        </a>
        <a href="#get-started" className="compare-btn compare-btn--premium" onClick={scrollToSignup}>
          Sign up to unlock Premium
        </a>
      </div>
    </div>
  );
}

function Home() {
  return (
    <main>
      <div className="hero-wrap">
        <h1>gameroom</h1>
        <p>Explore your Steam playtime with a public profile and daily heatmap.</p>
        <p className="tagline">See playtime changes observed since tracking began.</p>
        <div id="get-started">
          <Account />
        </div>
      </div>
      <SampleProfile />
      <FeatureComparison />
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
