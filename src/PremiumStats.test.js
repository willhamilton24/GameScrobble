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
const typeSearch = async value => {
  await act(async () => { Simulate.change(root.querySelector('input[type="search"]'), { target: { value } }); });
};
const setActiveOnly = async checked => {
  await act(async () => { Simulate.change(root.querySelector('input[type="checkbox"]'), { target: { checked } }); });
};

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
  expect(fetch.mock.calls[0]).toEqual(['/u/stats', {
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
  expect(root.querySelector('[role=alert]').textContent).toContain("load your stats");
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

it('filters games by name search, keeps overall totals and tracked-game count unchanged', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  await typeSearch('Moon');
  expect(names()).toEqual(['Moon Harbor']);
  expect(root.querySelector('.stats-total').textContent).toContain('21h 5m');
  expect(root.textContent).toContain('3 tracked games');
});

it('name search is case-insensitive and preserves the selected sort', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  await click('Game'); // ascending by name
  await typeSearch('a'); // matches Amber Valley and Moon Harbor (contains 'a')
  expect(names()).toEqual(['Amber Valley', 'Moon Harbor']);
  await typeSearch('A'); // same case-insensitive result
  expect(names()).toEqual(['Amber Valley', 'Moon Harbor']);
});

it('active-in-last-two-weeks filter shows only games with recent playtime', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  await setActiveOnly(true);
  expect(names()).toEqual(['Amber Valley']);
  expect(root.querySelector('.stats-total').textContent).toContain('21h 5m');
  expect(root.textContent).toContain('3 tracked games');
});

it('shows distinct no-match state and no table when filters exclude all games', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  await typeSearch('zzz');
  expect(root.querySelector('table')).toBe(null);
  expect(root.querySelector('[role=status]').textContent).toContain('No games match');
  expect(root.querySelector('.stats-total')).not.toBe(null);
});

it('clear filters restores all games and hides the clear button when inactive', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  expect(Array.from(root.querySelectorAll('button')).find(b => b.textContent.includes('Clear filters'))).toBeUndefined();
  await typeSearch('Amber');
  expect(names()).toEqual(['Amber Valley']);
  expect(Array.from(root.querySelectorAll('button')).find(b => b.textContent.includes('Clear filters'))).toBeTruthy();
  await click('Clear filters');
  expect(names()).toHaveLength(3);
  expect(Array.from(root.querySelectorAll('button')).find(b => b.textContent.includes('Clear filters'))).toBeUndefined();
});

it('hides search and filter controls for an empty account', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.empty));
  await render();
  expect(root.querySelector('input[type="search"]')).toBe(null);
  expect(root.querySelector('input[type="checkbox"]')).toBe(null);
});

it('active filter shows no-match state for a zero-recent-playtime account', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.zeroRecent));
  await render();
  expect(names()).toHaveLength(2);
  await setActiveOnly(true);
  expect(root.querySelector('table')).toBe(null);
  expect(root.querySelector('[role=status]').textContent).toContain('No games match');
  expect(root.querySelector('.stats-total')).not.toBe(null);
  await setActiveOnly(false);
  expect(names()).toHaveLength(2);
});
