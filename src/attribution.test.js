import { captureAttribution, signupAttribution } from './attribution';

beforeEach(() => { sessionStorage.clear(); window.history.replaceState({}, '', '/'); });

test('initial campaign survives navigation to registration and later campaign links', () => {
  window.history.replaceState({}, '', '/?utm_source=reddit_patientgamers');
  captureAttribution();
  window.history.replaceState({}, '', '/account');
  captureAttribution();
  expect(signupAttribution()).toBe('reddit_patientgamers');
  window.history.replaceState({}, '', '/?utm_source=backloggd');
  captureAttribution();
  expect(signupAttribution()).toBe('reddit_patientgamers');
});

test('direct initial entry remains direct', () => {
  captureAttribution();
  window.history.replaceState({}, '', '/account');
  expect(signupAttribution()).toBe('direct');
});

test('malformed and oversized campaigns become unknown without truncation', () => {
  ['é', 'a'.repeat(10000), '<script>', 'backloggd\n'].forEach(source => {
    sessionStorage.clear();
    window.history.replaceState({}, '', '/?utm_source=' + encodeURIComponent(source));
    captureAttribution();
    expect(signupAttribution()).toBe('unknown');
  });
});
