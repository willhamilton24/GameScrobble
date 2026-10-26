import React from 'react';
import ReactDOM from 'react-dom';
import { act, Simulate } from 'react-dom/test-utils';
import ProfilePage from './ProfilePage';
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
