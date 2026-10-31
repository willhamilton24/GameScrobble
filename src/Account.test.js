import React from 'react';
import ReactDOM from 'react-dom';
import { act, Simulate } from 'react-dom/test-utils';
import Account from './Account';
import { captureAttribution } from './attribution';

let root;
const account = { id: 'qa-account', email: 'qa@example.test' };
const response = (body, status = 200) => Promise.resolve({ ok: status < 400, status, json: () => Promise.resolve(body) });
async function render() {
  await act(async () => { ReactDOM.render(<Account />, root); });
}
async function submit() {
  root.querySelector('[name=email]').value = account.email;
  root.querySelector('[name=password]').value = 'test-password-only';
  await act(async () => { Simulate.submit(root.querySelector('form')); });
}
beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  window.history.replaceState({}, '', '/');
  root = document.createElement('div');
  document.body.appendChild(root);
  global.fetch = jest.fn();
});
afterEach(() => {
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  root.remove();
  delete global.fetch;
});

it('signs up, restores the account after reload, logs out, and logs back in', async () => {
  await render();
  fetch.mockImplementationOnce(() => response({ token: 'test-token', user: account }, 201));
  await submit();
  expect(fetch.mock.calls[0][0]).toBe('/auth/register');
  expect(root.textContent).toContain(`Signed in as ${account.email}`);
  expect(localStorage.getItem('gameroom.token')).toBe('test-token');
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  fetch.mockImplementationOnce(() => response(account));
  await render();
  // calls[0]=register, calls[1]=premium/status (post-register), calls[2]=/auth/me (restore)
  expect(fetch.mock.calls[2]).toEqual(['/auth/me', { headers: { Authorization: 'Bearer test-token' } }]);
  expect(root.textContent).toContain(account.email);
  act(() => { Simulate.click(Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Log out')); });
  expect(localStorage.getItem('gameroom.token')).toBe(null);
  expect(root.textContent).not.toContain('Signed in as');
  fetch.mockImplementationOnce(() => response({ token: 'new-test-token', user: account }));
  await submit();
  // calls[0]=register, [1]=premium/status, [2]=/auth/me, [3]=premium/status, [4]=login
  expect(fetch.mock.calls[4][0]).toBe('/auth/login');
  expect(root.textContent).toContain(account.email);
  expect(localStorage.getItem('gameroom.token')).toBe('new-test-token');
});

it('discards an expired session and displays authentication errors', async () => {
  localStorage.setItem('gameroom.token', 'expired-test-token');
  fetch.mockImplementationOnce(() => response({ error: 'invalid or expired token' }, 401));
  await render();
  expect(localStorage.getItem('gameroom.token')).toBe(null);
  fetch.mockImplementationOnce(() => response({ error: 'email already registered' }, 409));
  await submit();
  expect(root.querySelector('[role=alert]').textContent).toBe('email already registered');
  expect(root.textContent).not.toContain('Signed in as');
});

 it('sends initial entry attribution after navigating to the registration page', async () => {
  window.history.replaceState({}, '', '/u/example?utm_source=backloggd');
  captureAttribution();
  window.history.replaceState({}, '', '/');
  captureAttribution();
  await render();
  fetch.mockImplementationOnce(() => response({ token: 'test-token', user: account }, 201));
  await submit();
  expect(JSON.parse(fetch.mock.calls[0][1].body).source).toBe('backloggd');
 });

it('retains the stored token and shows a retry button on a 5xx server error', async () => {
  localStorage.setItem('gameroom.token', 'stored-token');
  fetch.mockImplementationOnce(() => response({ error: 'internal server error' }, 500));
  await render();
  expect(localStorage.getItem('gameroom.token')).toBe('stored-token');
  expect(root.querySelector('[role=alert]').textContent).toContain('Please try again');
  const retryBtn = Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Retry');
  expect(retryBtn).not.toBeNull();
  expect(root.textContent).not.toContain('Signed in as');
});

it('retains the stored token and shows a retry button on a network failure', async () => {
  localStorage.setItem('gameroom.token', 'stored-token');
  fetch.mockImplementationOnce(() => Promise.reject(new Error('Failed to fetch')));
  await render();
  expect(localStorage.getItem('gameroom.token')).toBe('stored-token');
  expect(root.querySelector('[role=alert]').textContent).toContain('Please try again');
  const retryBtn = Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Retry');
  expect(retryBtn).not.toBeNull();
});

it('restores the session on retry without requiring re-login', async () => {
  localStorage.setItem('gameroom.token', 'stored-token');
  fetch.mockImplementationOnce(() => response({ error: 'gateway timeout' }, 504));
  await render();
  expect(root.textContent).not.toContain('Signed in as');
  expect(localStorage.getItem('gameroom.token')).toBe('stored-token');

  fetch.mockImplementationOnce(() => response(account));
  const retryBtn = Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Retry');
  await act(async () => { Simulate.click(retryBtn); });
  expect(fetch.mock.calls[1]).toEqual(['/auth/me', { headers: { Authorization: 'Bearer stored-token' } }]);
  expect(root.textContent).toContain(`Signed in as ${account.email}`);
  expect(root.querySelector('[role=alert]')).toBeNull();
});

it('prevents duplicate retries: Retry button is absent while a retry is in progress', async () => {
  localStorage.setItem('gameroom.token', 'stored-token');
  fetch.mockImplementationOnce(() => response({ error: 'bad gateway' }, 502));
  await render();
  const retryBtn = () => Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Retry');
  expect(retryBtn()).toBeDefined();

  let resolve;
  fetch.mockImplementationOnce(() => new Promise(res => { resolve = res; }));
  await act(async () => { Simulate.click(retryBtn()); });
  // Button disappears once the retry is in flight — can't trigger a duplicate
  expect(retryBtn()).toBeUndefined();
  await act(async () => { resolve({ ok: true, status: 200, json: () => Promise.resolve(account) }); });
  expect(root.textContent).toContain(`Signed in as ${account.email}`);
});

it('ignores a stale /auth/me response if the token was cleared before the response arrived', async () => {
  localStorage.setItem('gameroom.token', 'stored-token');
  let resolve;
  fetch.mockImplementationOnce(() => new Promise(res => { resolve = res; }));
  act(() => { ReactDOM.render(<Account />, root); });
  // Clear the token (simulating logout from another tab) before the response arrives
  localStorage.removeItem('gameroom.token');
  await act(async () => {
    resolve({ ok: true, status: 200, json: () => Promise.resolve(account) });
  });
  expect(root.textContent).not.toContain('Signed in as');
});

it('discards an expired session (genuine 401) without offering a retry', async () => {
  localStorage.setItem('gameroom.token', 'expired-token');
  fetch.mockImplementationOnce(() => response({ error: 'invalid or expired token' }, 401));
  await render();
  expect(localStorage.getItem('gameroom.token')).toBe(null);
  const retryBtn = Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Retry');
  expect(retryBtn).toBeUndefined();
});

it('opens stats for the signed-in account and removes them on logout', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  fetch.mockImplementationOnce(() => response(account));
  await render();
  fetch.mockImplementationOnce(() => response({ stats: [] }));
  await act(async () => {
    Simulate.click(Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'View Premium stats'));
  });
  // calls[0]=/auth/me (restore), calls[1]=premium/status (post-restore), calls[2]=/u/stats
  expect(fetch.mock.calls[2]).toEqual(['/u/stats', {
    headers: { Authorization: 'Bearer fixture-token' }, cache: 'no-store'
  }]);
  expect(root.textContent).toContain('No game stats yet');
  act(() => { Simulate.click(Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Log out')); });
  expect(root.querySelector('.premium-stats')).toBe(null);
});

it('shows a payment-issue alert when premium status reports payment_problem', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  fetch.mockImplementationOnce(() => response(account));
  fetch.mockImplementationOnce(() => response({ premium: true, payment_problem: true, user_id: account.id }));
  await render();
  const alert = root.querySelector('.payment-issue');
  expect(alert).not.toBeNull();
  expect(alert.textContent).toMatch(/payment/i);
  expect(alert.textContent).toMatch(/update your payment method/i);
});

it('does not show a payment-issue alert when payment_problem is false', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  fetch.mockImplementationOnce(() => response(account));
  fetch.mockImplementationOnce(() => response({ premium: true, payment_problem: false, user_id: account.id }));
  await render();
  expect(root.querySelector('.payment-issue')).toBeNull();
});

it('does not show a payment-issue alert when the subscriber is no longer premium', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  fetch.mockImplementationOnce(() => response(account));
  fetch.mockImplementationOnce(() => response({ premium: false, payment_problem: true, user_id: account.id }));
  await render();
  expect(root.querySelector('.payment-issue')).toBeNull();
});

it('clears the payment-issue alert on logout', async () => {
  localStorage.setItem('gameroom.token', 'fixture-token');
  fetch.mockImplementationOnce(() => response(account));
  fetch.mockImplementationOnce(() => response({ premium: true, payment_problem: true, user_id: account.id }));
  await render();
  expect(root.querySelector('.payment-issue')).not.toBeNull();
  act(() => { Simulate.click(Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Log out')); });
  expect(root.querySelector('.payment-issue')).toBeNull();
});
