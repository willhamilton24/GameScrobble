import React from 'react';
import ReactDOM from 'react-dom';
import { act } from 'react-dom/test-utils';
import GameDeals from './GameDeals';
import ProfilePage from './ProfilePage';
import PremiumStats from './PremiumStats';
import deals from './fixtures/deals.json';
import profiles from './fixtures/profile.json';
import stats from './fixtures/premiumStats.json';

let root;
const response = (body, status = 200) => Promise.resolve({ ok: status < 400, status, json: () => Promise.resolve(body) });
beforeEach(() => {
  root = document.createElement('div');
  document.body.appendChild(root);
  global.fetch = jest.fn();
});
afterEach(() => {
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  root.remove();
  delete global.fetch;
});

['profile', 'premium'].forEach(surface => {
  [
    ['offersPresent', 200, 'humble — PC (affiliate link)'],
    ['noOffers', 200, 'No current offers.'],
    ['untagged', 200, 'humble — PC (affiliate link)'],
    ['notFound', 404, 'No deals found for this game.'],
    ['lookupFailure', 500, 'Deals are temporarily unavailable.'],
    ['network', 0, 'Deals are temporarily unavailable.']
  ].forEach(([fixture, status, message]) => {
    it(`${surface}: isolates ${fixture} and retains playtime`, async () => {
      fetch.mockImplementation(url => {
        if (url.startsWith('/game/')) return fixture === 'network'
          ? Promise.reject(new Error('offline')) : response(deals[fixture], status);
        return response(surface === 'profile' ? profiles.populated : stats.populated);
      });
      await act(async () => {
        ReactDOM.render(surface === 'profile' ? <ProfilePage userId="fixture" /> :
          <PremiumStats userId="fixture" token="fixture" />, root);
      });
      expect(root.textContent).toContain('21h 5m');
      expect(root.textContent).toContain(message);
      const links = Array.from(root.querySelectorAll('.game-deals a'));
      if (status !== 200) expect(links).toHaveLength(0);
      else {
        expect(links.some(link => link.getAttribute('href') === deals[fixture].find_more_deals)).toBe(true);
        links.forEach(link => {
          expect(link.textContent).toContain('affiliate link');
          expect(link.rel).toContain('sponsored');
          expect(link.rel).toContain('noopener');
        });
        if (fixture !== 'noOffers') expect(links[0].getAttribute('href')).toBe(deals[fixture].pc_deals[0].deal_url);
      }
    });
  });
});

it('encodes titles and ignores stale lookup responses after a title change', async () => {
  let resolve;
  fetch.mockImplementationOnce(() => new Promise(done => { resolve = done; }))
    .mockImplementationOnce(() => response(deals.noOffers));
  await act(async () => { ReactDOM.render(<GameDeals title="A/B & C?" />, root); });
  expect(fetch).toHaveBeenCalledWith('/game/A%2FB%20%26%20C%3F/deals');
  expect(root.textContent).toContain('Loading deals');
  await act(async () => { ReactDOM.render(<GameDeals title="Other" />, root); });
  await act(async () => { resolve(await response(deals.offersPresent)); });
  expect(root.textContent).toContain('No current offers');
  expect(root.textContent).not.toContain('humble');
});

it('does not render unsafe offer or search URLs', async () => {
  fetch.mockImplementation(() => response({ pc_deals: [{ deal_url: 'javascript:alert(1)' }], find_more_deals: 'data:text/html,bad' }));
  await act(async () => { ReactDOM.render(<GameDeals title="Fixture" />, root); });
  expect(root.querySelector('a')).toBe(null);
});

it('limits Premium deal lookups to the five most-played games', async () => {
  const games = Array.from({ length: 7 }, (_, i) => ({
    app_id: i, name: `Game ${i}`, playtime_total: i * 60,
    playtime_recent: 0, last_polled: '2026-10-31T12:00:00Z'
  }));
  fetch.mockImplementation(url => url === '/u/stats' ? response({ stats: games }) : response(deals.noOffers));
  await act(async () => { ReactDOM.render(<PremiumStats userId="fixture" token="fixture" />, root); });
  expect(root.querySelectorAll('tbody tr')).toHaveLength(7);
  expect(root.querySelectorAll('.game-deals')).toHaveLength(5);
  expect(fetch.mock.calls.filter(([url]) => url.startsWith('/game/')).map(([url]) => url)).toEqual(
    [6, 5, 4, 3, 2].map(i => `/game/Game%20${i}/deals`)
  );
});
