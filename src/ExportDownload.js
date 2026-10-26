import React, { useState } from 'react';

export default function ExportDownload({ className }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function download() {
    setBusy(true);
    setError('');
    let url;
    let link;
    try {
      const token = localStorage.getItem('gameroom.token');
      if (!token) throw new Error('Sign in to download your history.');
      const response = await fetch('/u/export', {
        headers: { Authorization: `Bearer ${token}` }, cache: 'no-store'
      });
      if (!response.ok) throw new Error('Could not download history. Sign in and check that Premium is active, then try again.');
      const blob = await response.blob();
      url = URL.createObjectURL(blob);
      link = document.createElement('a');
      link.href = url;
      link.download = 'gameroom_history.csv';
      document.body.appendChild(link);
      link.click();
    } catch (err) {
      setError(err.message || 'Could not download history. Please try again.');
    } finally {
      if (link) link.remove();
      if (url) URL.revokeObjectURL(url);
      setBusy(false);
    }
  }

  return <React.Fragment>
    <button type="button" className={className} onClick={download} disabled={busy}>
      {busy ? 'Downloading…' : 'Download game history (CSV)'}
    </button>
    {error && <span role="alert">{error}</span>}
  </React.Fragment>;
}
