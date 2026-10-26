import React, { useEffect, useState } from 'react';

import PremiumStats from './PremiumStats';

import { signupAttribution } from './attribution';

const sessionKey = 'gameroom.token';

export default function Account({ onUserChange, premiumRequest = 0 }) {
  const [user, setUser] = useState(null);
  const [showStats, setShowStats] = useState(false);
  const [mode, setMode] = useState('register');
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let mounted = true;
    async function restore() {
      try {
        const token = localStorage.getItem(sessionKey);
        if (!token) return;
        const response = await fetch('/auth/me', {
          headers: { Authorization: `Bearer ${token}` }
        });
        if (response.status === 401 || response.status === 404) {
          localStorage.removeItem(sessionKey);
          return;
        }
        if (!response.ok) throw new Error('Unable to restore your session. Please try again.');
        const account = await response.json();
        if (mounted) setUser(account);
      } catch (err) {
        if (mounted) setError(err.message);
      } finally {
        if (mounted) setBusy(false);
      }
    }
    restore();
    return () => { mounted = false; };
  }, []);

  useEffect(() => {
    if (onUserChange) onUserChange(user);
  }, [user, onUserChange]);

  useEffect(() => {
    if (!premiumRequest || busy) return;
    if (!user) setMode('register');
    const target = document.getElementById(user ? 'premium-checkout' : 'account-email');
    if (target) {
      target.scrollIntoView({ behavior: 'smooth', block: 'center' });
      target.focus({ preventScroll: true });
    }
  }, [premiumRequest, user, busy]);

  async function authenticate(event) {
    event.preventDefault();
    const form = event.currentTarget;
    const email = form.elements.email.value.trim();
    const password = form.elements.password.value;
    setBusy(true);
    setError('');
    try {
      const body = { email, password };
      if (mode === 'register') {
        body.source = signupAttribution();
      }
      const response = await fetch(`/auth/${mode}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || 'Unable to sign in.');
      localStorage.setItem(sessionKey, result.token);
      form.reset();
      setUser(result.user);
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  function logout() {
    try {
      localStorage.removeItem(sessionKey);
      setUser(null);
      setShowStats(false);
      setError('');
      setMode('login');
    } catch (err) {
      setError('Unable to clear your session. Please clear this site’s browser storage.');
    }
  }

  async function checkout() {
    setBusy(true);
    setError('');
    try {
      const response = await fetch('/api/stripe/checkout', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${localStorage.getItem(sessionKey)}` },
        body: JSON.stringify({ user_id: user.id })
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || 'Unable to start checkout.');
      window.location.assign(result.url);
    } catch (err) {
      setError(err.message);
      setBusy(false);
    }
  }

  return (
    <section className="account" aria-label="Your account">
      <h2>{user ? 'Your account' : mode === 'register' ? 'Create your account' : 'Log in'}</h2>
      {premiumRequest > 0 && !user && <p>Create your account, then use Upgrade to Premium to continue to checkout.</p>}
      {error && <p role="alert">{error}</p>}
      {user ? (
        <div>
          <p>Signed in as {user.email}</p>
          <p><a href={`/u/${encodeURIComponent(user.id)}`}>View your profile</a></p>
          <p><a href={`/u/export?user_id=${encodeURIComponent(user.id)}`}>Export game snapshots (Premium)</a></p>
          <button id="premium-checkout" type="button" disabled={busy} onClick={checkout}>Upgrade to Premium</button>
          <button type="button" disabled={busy} onClick={logout}>Log out</button>
          <button type="button" aria-expanded={showStats} aria-controls="account-stats" onClick={() => setShowStats(!showStats)}>
            {showStats ? 'Hide Premium stats' : 'View Premium stats'}
          </button>
          <div id="account-stats">
            {showStats && <PremiumStats key={user.id} userId={user.id} token={localStorage.getItem(sessionKey)} onUpgrade={checkout} busy={busy} />}
          </div>
        </div>
      ) : (
        <form onSubmit={authenticate}>
          <fieldset disabled={busy}>
            <label htmlFor="account-email">Email</label>
            <input id="account-email" name="email" type="email" autoComplete="email" required />
            <label htmlFor="account-password">Password</label>
            <input id="account-password" name="password" type="password" minLength={8} maxLength={72}
              autoComplete={mode === 'register' ? 'new-password' : 'current-password'} required />
            <button type="submit">{mode === 'register' ? 'Sign up' : 'Log in'}</button>
            <button type="button" onClick={() => { setMode(mode === 'register' ? 'login' : 'register'); setError(''); }}>
              {mode === 'register' ? 'Already have an account? Log in' : 'Create an account'}
            </button>
          </fieldset>
        </form>
      )}
      {busy && <p role="status">Please wait…</p>}
    </section>
  );
}
