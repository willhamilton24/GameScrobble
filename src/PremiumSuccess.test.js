import React from 'react';
import ReactDOM from 'react-dom';
import { act } from 'react-dom/test-utils';
import { PremiumSuccess, PremiumCancel } from './App';

let root;
const ok = (body, status = 200) =>
  Promise.resolve({ ok: status < 400, status, json: () => Promise.resolve(body) });

beforeEach(() => {
  root = document.createElement('div');
  document.body.appendChild(root);
  global.fetch = jest.fn();
  localStorage.clear();
});

afterEach(() => {
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  root.remove();
  delete global.fetch;
  jest.useRealTimers();
});

// Advance fake timers then flush React state / microtasks.
async function tick() {
  act(() => { jest.runAllTimers(); });
  await act(async () => {});
}

// ── active state: server confirms premium immediately ─────────────────────────

it('active: shows Premium activated and export link when server confirms immediately', async () => {
  localStorage.setItem('gameroom.token', 'tok');
  fetch.mockReturnValueOnce(ok({ premium: true, user_id: 'u1' }));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.textContent).toContain('Premium activated');
  expect(root.querySelector('a[href*="export"]')).not.toBe(null);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0][0]).toBe('/api/premium/status');
});

// ── delayed-webhook: not-premium on first poll, premium on second ─────────────

it('delayed-webhook: polls until premium is confirmed after one retry', async () => {
  localStorage.setItem('gameroom.token', 'tok');
  fetch
    .mockReturnValueOnce(ok({ premium: false, user_id: 'u1' }))
    .mockReturnValueOnce(ok({ premium: true, user_id: 'u1' }));
  jest.useFakeTimers();
  await act(async () => { ReactDOM.render(<PremiumSuccess _pollIntervalMs={0} />, root); });
  // First poll returned not-premium; still polling
  expect(root.textContent).toContain('Activating');
  expect(root.querySelector('[role=status]')).not.toBe(null);
  // Fire the 0ms retry timer and flush
  await tick();
  expect(root.textContent).toContain('Premium activated');
  expect(fetch).toHaveBeenCalledTimes(2);
});

// ── cancelled / expired: all polls exhausted, never premium ──────────────────

it('expired: shows timed-out message when all polls return not-premium', async () => {
  localStorage.setItem('gameroom.token', 'tok');
  // _maxPolls=3 → 3 polls total (attempt 0, 1, 2); exhausted after attempt 2
  for (let i = 0; i < 3; i++) {
    fetch.mockReturnValueOnce(ok({ premium: false, user_id: 'u1' }));
  }
  jest.useFakeTimers();
  await act(async () => { ReactDOM.render(<PremiumSuccess _maxPolls={3} _pollIntervalMs={0} />, root); });
  // First poll done, two retries remain
  await tick(); // attempt 1
  await tick(); // attempt 2 — exhausted
  expect(root.textContent).toContain('taking longer');
  expect(fetch).toHaveBeenCalledTimes(3);
});

// ── cancelled screen ──────────────────────────────────────────────────────────

it('cancelled: cancel screen shows no-charge message and home link', () => {
  const div = document.createElement('div');
  ReactDOM.render(<PremiumCancel />, div);
  expect(div.textContent).toContain('Checkout cancelled');
  expect(div.textContent).toContain('cancelled');
  expect(div.querySelector('a[href="/"]')).not.toBe(null);
  ReactDOM.unmountComponentAtNode(div);
});

// ── unauthenticated: no session token ────────────────────────────────────────

it('unauthenticated: shows sign-in prompt when no session token is present', async () => {
  // no token in localStorage
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.textContent).toContain('Sign in');
  expect(fetch).not.toHaveBeenCalled();
});

// ── export only after activation ──────────────────────────────────────────────

it('export link is absent while still activating', async () => {
  localStorage.setItem('gameroom.token', 'tok');
  let resolve;
  fetch.mockReturnValueOnce(new Promise(r => { resolve = r; }));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.querySelector('a[href*="export"]')).toBe(null);
  // Resolve as premium and verify link appears
  await act(async () => { resolve(ok({ premium: true, user_id: 'u2' }).then(v => v)); });
  // Ensure the pending promise chain resolves
  await act(async () => {});
  expect(root.querySelector('a[href*="export"]')).not.toBe(null);
});

// ── server error ──────────────────────────────────────────────────────────────

it('error: shows alert when the status endpoint returns a server error', async () => {
  localStorage.setItem('gameroom.token', 'tok');
  fetch.mockReturnValueOnce(ok({ error: 'db error' }, 500));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.querySelector('[role=alert]')).not.toBe(null);
  expect(root.querySelector('a[href*="export"]')).toBe(null);
});
