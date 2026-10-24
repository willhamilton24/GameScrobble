import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';

it('renders the gameroom home page', () => {
  const div = document.createElement('div');
  ReactDOM.render(<App />, div);
  expect(div.querySelector('h1').textContent).toBe('gameroom');
  expect(div.querySelector('p').textContent).toBe(
    'Unified play-history tracking across Steam, Xbox, PSN, and Epic.'
  );
  ReactDOM.unmountComponentAtNode(div);
});
