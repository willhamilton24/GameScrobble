import React from 'react';
import ProfilePage from './ProfilePage';
import './App.css';

function Home() {
  return (
    <main>
      <h1>GameScrobble</h1>
      <p>Last.fm for games — unified play-history tracking across Steam, Xbox, PSN, and Epic.</p>
      <p className="tagline">Share your year in games. Track every session.</p>
      <p className="coming-soon">Public beta coming soon.</p>
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
