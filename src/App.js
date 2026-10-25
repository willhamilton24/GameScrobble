import React from 'react';
import ProfilePage from './ProfilePage';
import Account from './Account';
import './App.css';

function Home() {
  return (
    <main>
      <h1>GameScrobble</h1>
      <p>Explore your Steam playtime with a public profile and daily heatmap.</p>
      <p className="tagline">See playtime changes observed since tracking began.</p>
      <Account />
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
