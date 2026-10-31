import React, { useEffect, useState } from 'react';

import ExportDownload from './ExportDownload';
import PremiumStats from './PremiumStats';

import { signupAttribution } from './attribution';

const sessionKey = 'gameroom.token';

export default function Account({ onUserChange, premiumRequest = 0 }) {
  const [user, setUser] = useState(null);
  const [showStats, setShowStats] = useState(false);
  const [mode, setMode] = useState('register');
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState('');
  const [canRetry, setCanRetry] = useState(false);
  const [retryCount, setRetryCount] = useState(0);
  const [paymentProblem, setPaymentProblem] = useState(false);

  useEffect(() => {
    let mounted = true;
    const tokenAtStart = localStorage.getItem(sessionKey);

    if (!tokenAtStart) {
      if (mounted) setBusy(false);
      return () => { mounted = false; };
    }

    setBusy(true);
    setError('');
    setCanRetry(false);

    async function restore() {
      try {
        const response = await fetch('/auth/me', {
          headers: { Authorization: `Bearer ${tokenAtStart}` }
        });
        if (!mounted) return;
        // Stale: token was cleared by logout while fetch was in flight
        if (localStorage.getItem(sessionKey) !== tokenAtStart) return;
        if (response.status === 401 || response.status === 404) {
          localStorage.removeItem(sessionKey);
          return;
        }
        if (!response.ok) {
          setCanRetry(true);
          setError('Unable to restore your session. Please try again.');
          return;
        }
        const account = await response.json();
        setUser(account);
        try {
          const statusResp = await fetch('/api/premium/status', {
            headers: { Authorization: `Bearer ${tokenAtStart}` }
          });
          if (mounted && statusResp && statusResp.ok) {
            const statusData = await statusResp.json();
            setPaymentProblem(!!statusData.payment_problem);
          }
        } catch (_) {}
      } catch (err) {
        if (!mounted) return;
        // Network/transport failure: keep token so the user can retry
        if (localStorage.getItem(sessionKey) === tokenAtStart) {
          setCanRetry(true);
          setError('Unable to reach the server. Please try again.');
        }
      } finally {
        if (mounted) setBusy(false);
      }
    }

    restore();
    return () => { mounted = false; };
  }, [retryCount]);

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
      try {
        const statusResp = await fetch('/api/premium/status', {
          headers: { Authorization: `Bearer ${result.token}` }
        });
        if (statusResp && statusResp.ok) {
          const statusData = await statusResp.json();
          setPaymentProblem(!!statusData.payment_problem);
        }
      } catch (_) {}
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  function handleRetry() {
    if (busy) return;
    setBusy(true);
    setRetryCount(c => c + 1);
  }

  function logout() {
    try {
      localStorage.removeItem(sessionKey);
      setUser(null);
      setShowStats(false);
      setError('');
      setCanRetry(false);
      setPaymentProblem(false);
      setMode('login');
    } catch (err) {
      setError("Unable to clear your session. Please clear this site\u2019s browser storage.");
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
      {canRetry && <button type="button" disabled={busy} onClick={handleRetry}>Retry</button>}
      {user ? (
        <div>
          <p>Signed in as {user.email}</p>
          {paymentProblem && (
            <p role="alert" className="payment-issue">
              Your last payment failed. Please update your payment method to keep your Premium access.
              Visit your billing settings or contact support to resolve this.
            </p>
          )}
          <p><a href={`/u/${encodeURIComponent(user.id)}`}>View your profile</a></p>
          <p><ExportDownload /></p>
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
