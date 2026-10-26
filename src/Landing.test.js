import React from 'react';
import ReactDOM from 'react-dom';
import { act, Simulate } from 'react-dom/test-utils';
import App from './App';

let root;
const user = { id: 'sample-test', email: 'test@example.test' };
const response = body => Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
beforeEach(() => {
  localStorage.clear();
  window.history.replaceState({}, '', '/');
  root = document.createElement('div');
  document.body.appendChild(root);
  Element.prototype.scrollIntoView = jest.fn();
  global.fetch = jest.fn();
});
afterEach(() => {
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  root.remove();
  delete global.fetch;
  delete Element.prototype.scrollIntoView;
});

it.each([false, true])('routes Premium intent to the existing checkout control (signed in: %s)', async signedIn => {
  if (signedIn) {
    localStorage.setItem('gameroom.token', 'test-token');
    fetch.mockImplementationOnce(() => response(user));
  }
  await act(async () => ReactDOM.render(<App />, root));
  const cta = root.querySelector('.compare-btn--premium');
  expect(cta.textContent).toBe(signedIn ? 'Go to Premium checkout' : 'Sign up to continue to Premium checkout');
  act(() => Simulate.click(cta));
  if (!signedIn) {
    expect(document.activeElement.id).toBe('account-email');
    root.querySelector('[name=email]').value = user.email;
    root.querySelector('[name=password]').value = 'test-password-only';
    fetch.mockImplementationOnce(() => response({ token: 'test-token', user }));
    await act(async () => Simulate.submit(root.querySelector('form')));
    expect(fetch.mock.calls[0][0]).toBe('/auth/register');
  }
  expect(document.activeElement.id).toBe('premium-checkout');
  expect(fetch.mock.calls.some(call => call[0] === '/api/stripe/checkout')).toBe(false);
  // Exercise the existing checkout request without navigating to a payment site.
  fetch.mockImplementationOnce(() => Promise.resolve({ ok: false, json: () => Promise.resolve({ error: 'Checkout test response' }) }));
  await act(async () => Simulate.click(root.querySelector('#premium-checkout')));
  expect(fetch.mock.calls[fetch.mock.calls.length - 1]).toEqual(['/api/stripe/checkout', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: 'Bearer test-token' },
    body: JSON.stringify({ user_id: user.id }),
  }]);
  expect(root.querySelector('table thead th[scope=col]')).not.toBe(null);
  expect(root.querySelectorAll('tbody th[scope=row]')).toHaveLength(4);
  expect(root.querySelector('[aria-label="Not included"]')).not.toBe(null);
});
