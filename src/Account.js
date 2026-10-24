import React, { useEffect, useState } from 'react';

const sessionKey = 'gamescrobble.token';

export default function Account() {
  const [user, setUser] = useState(null);
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
        const utmSource = new URLSearchParams(window.location.search).get('utm_source');
        if (utmSource) body.source = utmSource.slice(0, 64);
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
      {error && <p role="alert">{error}</p>}
      {user ? (
        <div>
          <p>Signed in as {user.email}</p>
          <p><a href={`/u/${encodeURIComponent(user.id)}`}>View your profile</a></p>
          <p><a href={`/u/export?user_id=${encodeURIComponent(user.id)}`}>Export history (Premium)</a></p>
          <button type="button" disabled={busy} onClick={checkout}>Upgrade to Premium</button>
          <button type="button" disabled={busy} onClick={logout}>Log out</button>
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
