import React from 'react';
import ReactDOM from 'react-dom';
import { act, Simulate } from 'react-dom/test-utils';
import PremiumStats from './PremiumStats';
import fixtures from './fixtures/premiumStats.json';

let root;
let upgrade;
const response = (body, status = 200) => Promise.resolve({ ok: status < 400, status, json: () => Promise.resolve(body) });
const render = async (userId = 'fixture-account') => {
  await act(async () => { ReactDOM.render(<PremiumStats userId={userId} token="fixture-token" onUpgrade={upgrade} />, root); });
};
const click = async label => {
  await act(async () => { Simulate.click(Array.from(root.querySelectorAll('button')).find(button => button.textContent.includes(label))); });
};
const names = () => Array.from(root.querySelectorAll('tbody th')).map(cell => cell.textContent);

beforeEach(() => {
  root = document.createElement('div');
  document.body.appendChild(root);
  global.fetch = jest.fn();
  upgrade = jest.fn();
});
afterEach(() => {
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  root.remove();
  delete global.fetch;
});

it('shows loading, then fixture totals and numeric, name and date sorting in both directions', async () => {
  let resolve;
  fetch.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  await render();
  expect(root.querySelector('[role=status]').textContent).toContain('Loading');
  expect(fetch.mock.calls[0]).toEqual(['/u/stats?user_id=fixture-account', {
    headers: { Authorization: 'Bearer fixture-token' }, cache: 'no-store'
  }]);
  await act(async () => { resolve(await response(fixtures.populated)); });
  expect(root.textContent).toContain('All-time playtime: 21h 5m');
  expect(names()).toEqual(['Zenith', 'Amber Valley', 'Moon Harbor']);
  await click('All-time playtime');
  expect(names()).toEqual(['Moon Harbor', 'Amber Valley', 'Zenith']);
  expect(root.querySelectorAll('thead th')[1].getAttribute('aria-sort')).toBe('ascending');
  await click('All-time playtime');
  expect(names()[0]).toBe('Zenith');
  await click('Game');
  expect(names()).toEqual(['Amber Valley', 'Moon Harbor', 'Zenith']);
  await click('Game');
  expect(names()).toEqual(['Zenith', 'Moon Harbor', 'Amber Valley']);
  await click('Recent playtime');
  expect(names()[0]).toBe('Amber Valley');
  await click('Recent playtime');
  expect(names()[2]).toBe('Amber Valley');
  await click('Last synced');
  expect(names()).toEqual(['Zenith', 'Amber Valley', 'Moon Harbor']);
  await click('Last synced');
  expect(names()).toEqual(['Moon Harbor', 'Amber Valley', 'Zenith']);
});

it('renders a successful empty account with zero playtime and useful guidance', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.empty));
  await render();
  expect(root.textContent).toContain('All-time playtime: 0h 0m');
  expect(root.textContent).toContain('No game stats yet');
  expect(root.textContent).toContain('linked Steam account');
  expect(root.querySelector('table')).toBe(null);
  expect(root.querySelector('[role=alert]')).toBe(null);
});

it('offers the existing checkout action for 402 and can recheck entitlement', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.free, 402));
  await render();
  expect(root.textContent).toContain('Unlock all-time playtime');
  expect(root.querySelector('.stats-total')).toBe(null);
  await click('Upgrade to Premium');
  expect(upgrade).toHaveBeenCalledTimes(1);
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await click('Check access again');
  expect(root.querySelector('table')).not.toBe(null);
});

it.each(['server', 'network', 'malformed', 'unauthorized'])('retries a %s failure without presenting empty stats', async failure => {
  fetch.mockImplementationOnce(() => failure === 'network' ? Promise.reject(new Error('offline')) :
    failure === 'malformed' ? response({}) : response({ error: 'failure' }, failure === 'unauthorized' ? 401 : 500));
  await render();
  expect(root.querySelector('[role=alert]').textContent).toContain('couldn’t load');
  expect(root.querySelector('.stats-total')).toBe(null);
  fetch.mockImplementationOnce(() => response(fixtures.empty));
  await click('Retry stats');
  expect(root.textContent).toContain('No game stats yet');
});

it('ignores a stale response after switching accounts', async () => {
  let resolve;
  fetch.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  await render('first');
  fetch.mockImplementationOnce(() => response(fixtures.empty));
  await render('second');
  await act(async () => { resolve(await response(fixtures.populated)); });
  expect(root.textContent).toContain('No game stats yet');
  expect(root.textContent).not.toContain('Zenith');
});
