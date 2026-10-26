import React from 'react';
import ReactDOM from 'react-dom';
import { act, Simulate } from 'react-dom/test-utils';
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
  window.history.replaceState({}, '', '/');
  jest.restoreAllMocks();
  jest.useRealTimers();
});

// Advance fake timers then flush React state / microtasks.
async function tick() {
  act(() => { jest.runAllTimers(); });
  await act(async () => {});
}

// ── active state: server confirms premium immediately ─────────────────────────

it('active: shows Premium activated and export button when server confirms immediately', async () => {
  localStorage.setItem('gameroom.token', 'tok');
  fetch.mockReturnValueOnce(ok({ premium: true, user_id: 'u1' }));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.textContent).toContain('Premium activated');
  expect(root.querySelector('button')).not.toBe(null);
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
  await tick();
  expect(fetch).toHaveBeenCalledTimes(3);
  expect(root.querySelector('button')).toBe(null);
});

// ── cancelled screen ──────────────────────────────────────────────────────────

it('cancelled: cancel screen shows neutral return message and home link', () => {
  const div = document.createElement('div');
  ReactDOM.render(<PremiumCancel />, div);
  expect(div.textContent).toContain('Checkout cancelled');
  expect(div.textContent).toContain('does not confirm payment or premium status');
  expect(fetch).not.toHaveBeenCalled();
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

it('export button is absent while still activating', async () => {
  localStorage.setItem('gameroom.token', 'tok');
  let resolve;
  fetch.mockReturnValueOnce(new Promise(r => { resolve = r; }));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.querySelector('button')).toBe(null);
  // Resolve as premium and verify link appears
  await act(async () => { resolve(ok({ premium: true, user_id: 'u2' }).then(v => v)); });
  // Ensure the pending promise chain resolves
  await act(async () => {});
  expect(root.querySelector('button')).not.toBe(null);
});

// ── server error ──────────────────────────────────────────────────────────────

it.each([401, 500])('error: shows alert when status returns %s', async status => {
  localStorage.setItem('gameroom.token', 'tok');
  fetch.mockReturnValueOnce(ok({ error: 'failure' }, status));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  expect(root.querySelector('[role=alert]')).not.toBe(null);
  expect(root.querySelector('button')).toBe(null);
});

it('URL parameters never grant access or select the account', async () => {
  window.history.replaceState({}, '', '/premium/success?session_id=fake&premium=true&user_id=other');
  localStorage.setItem('gameroom.token', 'tok');
  fetch.mockReturnValueOnce(ok({ premium: false, user_id: 'self' }));
  await act(async () => { ReactDOM.render(<PremiumSuccess _maxPolls={1} />, root); });
  expect(root.querySelector('button')).toBe(null);
  expect(fetch.mock.calls[0]).toEqual(['/api/premium/status', {
    headers: { Authorization: 'Bearer tok' }, cache: 'no-store'
  }]);
});

it.each(['timer', 'request'])('unmount cleans up polling during a pending %s', async stage => {
  jest.useFakeTimers();
  localStorage.setItem('gameroom.token', 'tok');
  let resolve;
  fetch.mockReturnValueOnce(stage === 'timer' ? ok({ premium: false }) : new Promise(done => { resolve = done; }));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  if (resolve) await act(async () => { resolve(await ok({ premium: false })); });
  await tick();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(clearTimeout).toHaveBeenCalled();
});

async function activate() {
  localStorage.setItem('gameroom.token', 'tok');
  fetch.mockReturnValueOnce(ok({ premium: true, user_id: 'self' }));
  await act(async () => { ReactDOM.render(<PremiumSuccess />, root); });
  URL.createObjectURL = jest.fn(() => 'blob:fixture');
  URL.revokeObjectURL = jest.fn();
}

it('downloads with bearer authentication and revokes the blob URL', async () => {
  await activate();
  const blob = new Blob(['app_id,name\n1,Fixture']);
  fetch.mockResolvedValueOnce({ ok: true, blob: () => Promise.resolve(blob) });
  const clicked = jest.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function () {
    expect(this.download).toBe('gameroom_history.csv');
    expect(this.getAttribute('href')).toBe('blob:fixture');
  });
  await act(async () => { Simulate.click(root.querySelector('button')); });
  expect(fetch.mock.calls[1]).toEqual(['/u/export', {
    headers: { Authorization: 'Bearer tok' }, cache: 'no-store'
  }]);
  expect(clicked).toHaveBeenCalledTimes(1);
  expect(URL.createObjectURL).toHaveBeenCalledWith(blob);
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:fixture');
  expect(document.querySelector('a[download]')).toBe(null);
});

it.each([401, 402, 500, 'network'])('download failure %s shows an error without downloading', async failure => {
  await activate();
  const blob = jest.fn();
  if (failure === 'network') fetch.mockRejectedValueOnce(new Error('offline'));
  else fetch.mockResolvedValueOnce({ ok: false, status: failure, blob });
  const clicked = jest.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  await act(async () => { Simulate.click(root.querySelector('button')); });
  expect(root.querySelector('[role=alert]')).not.toBe(null);
  expect(root.querySelector('button').disabled).toBe(false);
  expect(blob).not.toHaveBeenCalled();
  expect(clicked).not.toHaveBeenCalled();
  expect(URL.createObjectURL).not.toHaveBeenCalled();
});
