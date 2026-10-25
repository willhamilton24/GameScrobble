const key = 'gamescrobble.signupSource';
const supported = ['direct', 'unknown', 'reddit_patientgamers', 'backloggd'];

// First application entry wins for this tab, including entry without a campaign.
export function captureAttribution() {
  try {
    if (sessionStorage.getItem(key) !== null) return;
    const raw = new URLSearchParams(window.location.search).get('utm_source');
    const source = raw === null || raw === '' ? 'direct' : supported.indexOf(raw) >= 0 ? raw : 'unknown';
    sessionStorage.setItem(key, source);
  } catch (_) { /* Storage may be unavailable; registration remains usable. */ }
}

export function signupAttribution() {
  try {
    const source = sessionStorage.getItem(key);
    return supported.indexOf(source) >= 0 ? source : 'unknown';
  } catch (_) { return 'unknown'; }
}
