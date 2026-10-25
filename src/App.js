import React from 'react';
import ProfilePage from './ProfilePage';
import Account from './Account';
import './App.css';

function Home() {
  return (
    <main>
      <h1>gameroom</h1>
      <p>Unified play-history tracking across Steam, Xbox, PSN, and Epic.</p>
      <p className="tagline">Share your year in games. Track every session.</p>
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
