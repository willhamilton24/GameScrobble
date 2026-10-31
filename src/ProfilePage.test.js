import React from 'react';
import ReactDOM from 'react-dom';
import { act, Simulate } from 'react-dom/test-utils';
import ProfilePage, { buildHeatmapWeeks } from './ProfilePage';
import fixtures from './fixtures/profile.json';

let root;
const response = (body, status = 200) => Promise.resolve({ ok: status < 400, status, json: () => Promise.resolve(body) });
const render = async (userId = 'fixture-user') => {
  await act(async () => { ReactDOM.render(<ProfilePage userId={userId} />, root); });
};
const click = async label => {
  await act(async () => { Simulate.click(Array.from(root.querySelectorAll('button')).find(b => b.textContent.includes(label))); });
};

beforeEach(() => {
  root = document.createElement('div');
  document.body.appendChild(root);
  global.fetch = jest.fn();
  delete navigator.clipboard;
});
afterEach(() => {
  act(() => { ReactDOM.unmountComponentAtNode(root); });
  root.remove();
  delete global.fetch;
});

it('shows a loading indicator while fetching', async () => {
  let resolve;
  fetch.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  await render();
  expect(root.querySelector('[role=status]').textContent).toContain('Loading');
  await act(async () => { resolve(await response(fixtures.populated)); });
});

it('renders a populated profile with stats and games', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  expect(root.textContent).toContain('Dota 2');
  expect(root.textContent).toContain('21h 5m');
  expect(root.querySelectorAll('.game-card').length).toBe(3);
  expect(root.querySelector('[role=alert]')).toBe(null);
});

it('renders an empty profile with zero activity message', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.empty));
  await render();
  expect(root.textContent).toContain('0m');
  expect(root.textContent).toContain('No gaming activity recorded yet');
  expect(root.querySelectorAll('.game-card').length).toBe(0);
});

it('shows not-found state for 404 with signup CTA and no retry', async () => {
  fetch.mockImplementationOnce(() => response({}, 404));
  await render('ghost-user');
  expect(root.textContent).toContain('No profile found');
  expect(root.textContent).toContain('ghost-user');
  expect(root.querySelector('a[href="/"]')).not.toBe(null);
  expect(root.querySelector('[role=alert]')).toBe(null);
  expect(Array.from(root.querySelectorAll('button')).find(b => b.textContent.includes('Retry'))).toBeUndefined();
});

it('shows a server error state with retry for 5xx responses', async () => {
  fetch.mockImplementationOnce(() => response({ error: 'internal' }, 500));
  await render();
  expect(root.querySelector('[role=alert]').textContent).toContain('Something went wrong');
  expect(root.textContent).toContain('couldn');
  expect(Array.from(root.querySelectorAll('button')).find(b => b.textContent.includes('Retry'))).toBeDefined();
});

it('shows a server error state with retry for network failures', async () => {
  fetch.mockImplementationOnce(() => Promise.reject(new Error('offline')));
  await render();
  expect(root.querySelector('[role=alert]').textContent).toContain('Something went wrong');
  expect(Array.from(root.querySelectorAll('button')).find(b => b.textContent.includes('Retry'))).toBeDefined();
});

it('retries and recovers from a server error', async () => {
  fetch.mockImplementationOnce(() => response({ error: 'internal' }, 500));
  await render();
  expect(root.querySelector('[role=alert]')).not.toBe(null);
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await click('Retry');
  expect(root.querySelector('[role=alert]')).toBe(null);
  expect(root.textContent).toContain('Dota 2');
});

it('not-found and error states are distinct: 404 shows signup CTA, 5xx shows retry', async () => {
  fetch.mockImplementationOnce(() => response({}, 404));
  await render('unknown-user');
  expect(root.textContent).toContain('No profile found');
  expect(root.textContent).not.toContain('Something went wrong');

  fetch.mockImplementationOnce(() => response({}, 500));
  await render('erroring-user');
  expect(root.textContent).toContain('Something went wrong');
  expect(root.textContent).not.toContain('No profile found');
});

it('copies the URL to clipboard and shows confirmation', async () => {
  let writeResolve;
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: jest.fn(() => new Promise(r => { writeResolve = r; })) },
    configurable: true,
  });
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  await click('Share');
  expect(navigator.clipboard.writeText).toHaveBeenCalledWith(window.location.href);
  expect(root.textContent).not.toContain('✓ Copied');
  await act(async () => { writeResolve(); });
  expect(root.textContent).toContain('✓ Copied');
  expect(root.querySelector('.share-url')).toBe(null);
});

it('shows a selectable URL input when clipboard is unavailable', async () => {
  // navigator.clipboard is deleted in beforeEach
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  expect(root.querySelector('.share-url')).toBe(null);
  await click('Share');
  const input = root.querySelector('.share-url');
  expect(input).not.toBe(null);
  expect(input.value).toBe(window.location.href);
  expect(input.readOnly).toBe(true);
  expect(root.textContent).not.toContain('✓ Copied');
});

it('shows a selectable URL input when clipboard write is denied', async () => {
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: jest.fn(() => Promise.reject(new Error('NotAllowedError'))) },
    configurable: true,
  });
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  await click('Share');
  await act(async () => {});
  const input = root.querySelector('.share-url');
  expect(input).not.toBe(null);
  expect(input.value).toBe(window.location.href);
});

// ── buildHeatmapWeeks pure-unit tests (UTC boundary, DST, year boundary, empty)

it('buildHeatmapWeeks: UTC boundary date 2026-01-01 appears in generated weeks', () => {
  const weeks = buildHeatmapWeeks([{ date: '2026-01-01', minutes: 90 }], '2026-10-31');
  const allDays = weeks.flat();
  const day = allDays.find(d => d.date === '2026-01-01');
  expect(day).toBeDefined();
  expect(day.minutes).toBe(90);
});

it('buildHeatmapWeeks: DST spring-forward 2026-03-08 and fall-back 2025-11-02 appear as distinct days', () => {
  const weeks = buildHeatmapWeeks(
    [{ date: '2025-11-02', minutes: 45 }, { date: '2026-03-08', minutes: 75 }],
    '2026-10-31'
  );
  const allDays = weeks.flat();
  const fallBack = allDays.find(d => d.date === '2025-11-02');
  const springFwd = allDays.find(d => d.date === '2026-03-08');
  expect(fallBack).toBeDefined();
  expect(fallBack.minutes).toBe(45);
  expect(springFwd).toBeDefined();
  expect(springFwd.minutes).toBe(75);
  expect(fallBack.date).not.toBe(springFwd.date);
});

it('buildHeatmapWeeks: Dec 31 and Jan 1 appear as distinct days across year boundary', () => {
  const weeks = buildHeatmapWeeks(
    [{ date: '2025-12-31', minutes: 30 }, { date: '2026-01-01', minutes: 60 }],
    '2026-10-31'
  );
  const allDays = weeks.flat();
  const dec31 = allDays.find(d => d.date === '2025-12-31');
  const jan1 = allDays.find(d => d.date === '2026-01-01');
  expect(dec31).toBeDefined();
  expect(dec31.minutes).toBe(30);
  expect(jan1).toBeDefined();
  expect(jan1.minutes).toBe(60);
  expect(dec31.date).not.toBe(jan1.date);
});

it('buildHeatmapWeeks: empty data produces 52 weeks of zero-minute days', () => {
  const weeks = buildHeatmapWeeks([], '2026-10-31');
  expect(weeks.length).toBeGreaterThanOrEqual(52);
  expect(weeks.flat().every(d => d.minutes === 0)).toBe(true);
});

// ── Accessible readout and window-label DOM tests

it('Activity section is labeled as a rolling window, separate from the calendar-year total', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.populated));
  await render();
  expect(root.textContent).toContain('Last 52 weeks');
  expect(root.textContent).toContain('played this year');
});

it('clicking a heatmap cell shows its date and playtime in the readout', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.utcBoundary));
  await render();
  const cell = Array.from(root.querySelectorAll('[role=gridcell]'))
    .find(el => (el.getAttribute('aria-label') || '').startsWith('2026-01-01'));
  expect(cell).not.toBe(null);
  await act(async () => { Simulate.click(cell); });
  const readout = root.querySelector('.heatmap-readout');
  expect(readout).not.toBe(null);
  expect(readout.textContent).toContain('2026-01-01');
  expect(readout.textContent).toContain('1h 30m');
});

it('focusing a heatmap cell shows its date in the readout', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.utcBoundary));
  await render();
  const cell = Array.from(root.querySelectorAll('[role=gridcell]'))
    .find(el => (el.getAttribute('aria-label') || '').startsWith('2026-01-01'));
  await act(async () => { Simulate.focus(cell); });
  const readout = root.querySelector('.heatmap-readout');
  expect(readout).not.toBe(null);
  expect(readout.textContent).toContain('2026-01-01');
});

it('heatmap readout shows "No activity" for an empty cell', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.empty));
  await render();
  const cells = Array.from(root.querySelectorAll('[role=gridcell]'));
  expect(cells.length).toBeGreaterThan(0);
  await act(async () => { Simulate.click(cells[0]); });
  const readout = root.querySelector('.heatmap-readout');
  expect(readout).not.toBe(null);
  expect(readout.textContent).toContain('No activity');
});

it('year-boundary fixture renders both Dec 31 and Jan 1 cells', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.yearBoundary));
  await render();
  const cells = Array.from(root.querySelectorAll('[role=gridcell]'));
  const dec31 = cells.find(el => (el.getAttribute('aria-label') || '').startsWith('2025-12-31'));
  const jan1 = cells.find(el => (el.getAttribute('aria-label') || '').startsWith('2026-01-01'));
  expect(dec31).toBeDefined();
  expect(jan1).toBeDefined();
});

it('DST boundary fixture renders spring-forward and fall-back cells', async () => {
  fetch.mockImplementationOnce(() => response(fixtures.dstBoundary));
  await render();
  const cells = Array.from(root.querySelectorAll('[role=gridcell]'));
  const fallBack = cells.find(el => (el.getAttribute('aria-label') || '').startsWith('2025-11-02'));
  const springFwd = cells.find(el => (el.getAttribute('aria-label') || '').startsWith('2026-03-08'));
  expect(fallBack).toBeDefined();
  expect(springFwd).toBeDefined();
});
