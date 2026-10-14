/* Seraph board — keyboard-first, and a refresh that never takes the page out from
   under you. The board is only re-rendered when it actually changed, so typing in a
   field, hovering a row, or reading a description survives every poll. */
(function () {
  'use strict';

  var token  = new URLSearchParams(location.search).get('t');
  var host   = document.getElementById('board');
  var pulse  = document.getElementById('pulse');
  var filter = document.getElementById('filter');
  var count  = document.getElementById('count');
  var help   = document.getElementById('help');
  var helpOn = document.getElementById('help-toggle');
  var sel    = 0;
  var lastHash = null;
  var failures = 0;

  function cards() { return Array.prototype.slice.call(host.querySelectorAll('.card')); }

  function visible() {
    var q = filter.value.trim().toLowerCase();
    return cards().filter(function (r) {
      if (!q) { r.hidden = false; return true; }
      var hit = (r.dataset.t || '').toLowerCase().indexOf(q) > -1;
      r.hidden = !hit;
      return hit;
    });
  }

  function mark() {
    var shown = visible();
    count.textContent = filter.value.trim()
      ? shown.length + ' of ' + cards().length
      : '';
    shown.forEach(function (r, i) {
      r.dataset.sel = (i === sel) ? '1' : '0';
    });
  }

  function move(d) {
    var shown = visible();
    if (!shown.length) { return; }
    sel = Math.max(0, Math.min(shown.length - 1, sel + d));
    mark();
    shown[sel].focus();
  }

  function current() {
    if (document.activeElement && document.activeElement.classList.contains('card')) {
      return document.activeElement;
    }
    return visible()[sel] || null;
  }

  /* Fire the button that belongs to a key, so the keyboard path and the mouse path
     are literally the same request. */
  function act(key) {
    var card = current();
    if (!card) { return; }
    var btn = card.querySelector('.acts button[data-k="' + key + '"]');
    if (btn) { btn.form.submit(); }
  }

  function showHelp(open) {
    help.hidden = !open;
    helpOn.setAttribute('aria-expanded', String(open));
  }

  /* The side panel is the detail view. It is a panel, not a card copy: its header
     carries the signal, the id, the holder, and the title as its heading, and the
     body renders the task's own .detail fields — cloned, so the poll can keep the
     board fresh without touching what the user is reading. The in-card .detail
     block stays hidden; it is the data source, never a shown surface. */
  var side = document.getElementById('side');

  function showDetail(card) {
    var d = card.querySelector('.detail');
    side.innerHTML = '';
    side.className = 'side ' + (card.className.match(/aspect-[a-z]+/g) || []).join(' ');
    side.dataset.task = card.dataset.task;
    if (card.dataset.pri) { side.dataset.pri = card.dataset.pri; }

    var head = document.createElement('header');
    head.className = 'side-head';
    var meta = document.createElement('div');
    meta.className = 'side-meta';
    var sig = document.createElement('span');
    sig.className = 'signal';
    sig.setAttribute('aria-hidden', 'true');
    var id = document.createElement('span');
    id.className = 'side-id';
    id.textContent = card.dataset.task;
    meta.append(sig, id);
    var holder = card.querySelector('.card-top .holder');
    if (holder) { meta.appendChild(holder.cloneNode(true)); }
    var close = document.createElement('button');
    close.type = 'button';
    close.className = 'side-close';
    close.textContent = 'esc';
    close.setAttribute('aria-label', 'close detail');
    close.addEventListener('click', function () { side.hidden = true; });
    meta.appendChild(close);
    var title = document.createElement('h2');
    title.className = 'side-title';
    title.textContent = card.querySelector('.card-title').textContent;
    head.append(meta, title);

    var body = document.createElement('div');
    body.className = 'side-body';
    if (d) { var cd = d.cloneNode(true); cd.hidden = false; body.appendChild(cd); }
    side.append(head, body);
    side.hidden = false;
  }

  function toggleDetail() {
    var card = current();
    if (!card) { return; }
    if (!side.hidden && side.dataset.task === card.dataset.task) {
      side.hidden = true;
      return;
    }
    showDetail(card);
  }

  /* Focus is the selection. Anything that focuses a card adopts it, so the keys act on
     the card the user is actually on. */
  host.addEventListener('focusin', function (e) {
    var card = e.target.closest('.card');
    if (!card) { return; }
    sel = Math.max(0, visible().indexOf(card));
  });

  /* A click on a card opens its detail in the side panel; clicking another card
     moves it there. Buttons and forms inside the card keep their own behaviour. */
  host.addEventListener('click', function (e) {
    if (e.target.closest('button, a, input, form')) { return; }
    var card = e.target.closest('.card');
    if (!card) { return; }
    sel = Math.max(0, visible().indexOf(card));
    mark();
    card.focus();
    showDetail(card);
  });

  document.addEventListener('keydown', function (e) {
    if (e.target.closest('input, textarea, select')) { return; }
    switch (e.key) {
      case 'j': case 'ArrowDown': e.preventDefault(); move(1); break;
      case 'k': case 'ArrowUp':   e.preventDefault(); move(-1); break;
      case 'n': e.preventDefault(); openCompose(); break;
      case 'Enter': {
        var card = current();
        if (card) { toggleDetail(); }
        break;
      }
      case 'Escape': clear(); break;
      case '?': showHelp(help.hidden); break;
    }
  });

  /* The composer is a native <dialog>; the button in the rail and the n key open
     the same one, and Escape is the dialog's own. */
  var compose = document.getElementById('compose');
  var createOpen = document.getElementById('create-open');
  function openCompose() {
    if (compose.open) { return; }
    compose.showModal();
    compose.querySelector('input[name="title"]').focus();
  }
  if (createOpen) { createOpen.addEventListener('click', openCompose); }

  /* Escape clears the dialog first, then the filter, then the side panel, then
     focuses the filter — one key walks back out of whatever depth the user is in. */
  function clear() {
    if (compose.open) { compose.close(); return; }
    if (filter.value) { filter.value = ''; mark(); return; }
    if (!side.hidden) { side.hidden = true; return; }
    filter.focus();
  }

  /* A changed board is swapped in wholesale. The side panel lives outside #board, so
     an open detail survives every poll untouched. */
  function adopt(html) {
    host.innerHTML = html;
    var hash = host.querySelector('[data-hash]');
    lastHash = hash ? hash.dataset.hash : null;
    mark();
  }

  /* The pip is state, not decoration: it fills when a poll landed and reports stale when
     the server stops answering. */
  function markLive() {
    pulse.classList.remove('down');
    pulse.dataset.state = failures ? 'stale' : 'live';
  }

  function poll() {
    fetch('/board?t=' + encodeURIComponent(token))
      .then(function (r) { return r.ok ? r.text() : Promise.reject(r.status); })
      .then(function (html) {
        failures = 0;
        var m = html.match(/data-hash="([^"]*)"/);
        var h = m ? m[1] : null;
        if (h !== lastHash) {
          lastHash = h;
          adopt(html);
        }
        markLive();
      })
      .catch(function () {
        failures++;
        pulse.classList.add('down');
        pulse.dataset.state = 'stale';
      });
  }

  var hash = host.querySelector('[data-hash]');
  lastHash = hash ? hash.dataset.hash : null;

  mark();
  setInterval(poll, 1500);
})();
