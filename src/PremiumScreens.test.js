import React from 'react';
import ReactDOM from 'react-dom';
import { act } from 'react-dom/test-utils';
import { PremiumSuccess, PremiumCancel } from './App';

let root;
const account = { id: 'test-user-id', email: 'qa@example.test' };
const response = (body, status = 200) =>
  Promise.resolve({ ok: status < 400, status, json: () => Promise.resolve(body) });

beforeEach(() => {
  localStorage.clear();
  root = document.createElement('div');
  document.body.appendChild(root);
  global.fetch = jest.fn();
});

afterEach(() => {
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  root.remove();
  delete global.fetch;
  jest.useRealTimers();
});

// ── Cancel screen ────────────────────────────────────────────────

it('cancel: shows no-charge message with home link', async () => {
  await act(async () => { ReactDOM.render(<PremiumCancel />, root); });
  expect(root.textContent).toContain('Checkout cancelled');
  expect(root.textContent).toContain('No charge was made');
  expect(root.querySelector('a[href="/"]')).not.toBeNull();
  expect(global.fetch).not.toHaveBeenCalled();
});

// ── Success screen: unauthenticated ─────────────────────────────

it('success: shows sign-in prompt when no token is stored', async () => {
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.textContent).toContain('Sign in');
  expect(root.querySelector('a[href="/"]')).not.toBeNull();
  expect(global.fetch).not.toHaveBeenCalled();
});

it('success: shows error when the status endpoint rejects a bad token', async () => {
  localStorage.setItem('gameroom.token', 'bad-token');
  fetch.mockImplementationOnce(() => response({ error: 'invalid token' }, 401));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.querySelector('[role=alert]')).not.toBeNull();
  expect(fetch.mock.calls[0][0]).toBe('/api/premium/status');
});

// ── Success screen: active (webhook arrived before redirect) ────

it('success: shows export link when premium is already active', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  // Single call to /api/premium/status returns premium=true with user_id
  fetch.mockImplementationOnce(() => response({ premium: true, user_id: account.id }));

  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });

  expect(fetch.mock.calls[0][0]).toBe('/api/premium/status');
  expect(fetch).toHaveBeenCalledTimes(1);

  expect(root.textContent).toContain('Premium activated');
  const exportLink = root.querySelector(`a[href="/u/export?user_id=${encodeURIComponent(account.id)}"]`);
  expect(exportLink).not.toBeNull();
  // session_id in the URL was never used to grant access — status came from the server
});

// ── Success screen: delayed webhook (pending then activates) ─────

it('success: shows pending then activates when webhook arrives late', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  fetch.mockImplementationOnce(() => response({ premium: false, user_id: account.id })); // first poll
  fetch.mockImplementationOnce(() => response({ premium: true, user_id: account.id }));  // second poll

  jest.useFakeTimers();

  await act(async () => { ReactDOM.render(<PremiumSuccess _pollIntervalMs={100} />, root); });

  // After the first poll returns inactive the component must show pending, not the export link
  expect(root.textContent).toContain('Activating');
  expect(root.querySelector(`a[href^="/u/export"]`)).toBeNull();

  // Advance timer so the second poll fires, then flush its async result
  act(() => { jest.runAllTimers(); });
  await act(async () => {});

  expect(root.textContent).toContain('Premium activated');
  expect(root.querySelector(`a[href^="/u/export"]`)).not.toBeNull();
});

// ── Success screen: expired / timed-out (max polls exhausted) ───

it('success: shows timed-out message after all polls return inactive', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  // _maxPolls=1: the very first inactive response exhausts the poll budget
  fetch.mockImplementationOnce(() => response({ premium: false, user_id: account.id }));

  await act(async () => { ReactDOM.render(<PremiumSuccess _maxPolls={1} />, root); });

  expect(root.textContent).toContain('taking longer');
  expect(root.querySelector(`a[href^="/u/export"]`)).toBeNull();
});

// ── Export link is gated on server confirmation, not the URL ────

it('success: never shows export link while status is inactive regardless of URL params', async () => {
  // Simulate arriving with a session_id query param (as Stripe redirects)
  window.history.replaceState({}, '', '/premium/success?session_id=cs_test_fake');
  localStorage.setItem('gameroom.token', 'fixture-token');
  fetch.mockImplementationOnce(() => response({ premium: false, user_id: account.id }));

  await act(async () => { ReactDOM.render(<PremiumSuccess _maxPolls={1} />, root); });

  // Even with session_id present, export is not unlocked from URL alone
  expect(root.querySelector(`a[href^="/u/export"]`)).toBeNull();

  window.history.replaceState({}, '', '/');
});
