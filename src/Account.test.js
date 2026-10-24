import React from 'react';
import ReactDOM from 'react-dom';
import { act, Simulate } from 'react-dom/test-utils';
import Account from './Account';

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
  expect(fetch.mock.calls[1]).toEqual(['/auth/me', { headers: { Authorization: 'Bearer test-token' } }]);
  expect(root.textContent).toContain(account.email);
  act(() => { Simulate.click(Array.from(root.querySelectorAll('button')).find(b => b.textContent === 'Log out')); });
  expect(localStorage.getItem('gameroom.token')).toBe(null);
  expect(root.textContent).not.toContain('Signed in as');
  fetch.mockImplementationOnce(() => response({ token: 'new-test-token', user: account }));
  await submit();
  expect(fetch.mock.calls[2][0]).toBe('/auth/login');
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
