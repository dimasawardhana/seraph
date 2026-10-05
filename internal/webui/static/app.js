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

  function rows() { return Array.prototype.slice.call(host.querySelectorAll('.row')); }

  function visible() {
    var q = filter.value.trim().toLowerCase();
    return rows().filter(function (r) {
      if (!q) { r.hidden = false; return true; }
      var hit = (r.dataset.t || '').toLowerCase().indexOf(q) > -1;
      r.hidden = !hit;
      return hit;
    });
  }

  function mark() {
    var shown = visible();
    count.textContent = filter.value.trim()
      ? shown.length + ' of ' + rows().length
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
    /* Enter and the action keys must act on the row the user is actually on. Focusing a
       row — by click, by Tab, or by a screen reader — moves the caret without moving the
       selection index, so the focused row wins when there is one. */
    if (document.activeElement && document.activeElement.classList.contains('row')) {
      return document.activeElement;
    }
    return visible()[sel] || null;
  }

  /* Fire the button that belongs to a key, so the keyboard path and the mouse path
     are literally the same request. */
  function act(key) {
    var row = current();
    if (!row) { return; }
    var btn = row.querySelector('.acts button[data-k="' + key + '"]');
    if (btn) { btn.form.submit(); }
  }

  function showHelp(open) {
    help.hidden = !open;
    helpOn.setAttribute('aria-expanded', String(open));
  }



  function toggleDetail() {
    var row = current();
    if (!row) { return; }
    var d = row.querySelector('.detail');
    if (d) { d.hidden = !d.hidden; }
  }

  /* Focus is the selection. Anything that focuses a row adopts it, so the keys act on
     the row the user is actually on. */
  host.addEventListener('focusin', function (e) {
    var row = e.target.closest('.row');
    if (!row) { return; }
    sel = Math.max(0, visible().indexOf(row));
    mark();
  });
  function clear() {
    if (filter.value) { filter.value = ''; mark(); return; }
    var open = host.querySelector('.detail:not([hidden])');
    if (open) { open.hidden = true; return; }
    filter.focus();
  }

  document.addEventListener('keydown', function (e) {
    var typing = /^(INPUT|SELECT|TEXTAREA)$/.test(e.target.tagName);

    if (e.key === 'Escape') { clear(); if (typing) e.target.blur(); return; }
    if (typing || e.metaKey || e.ctrlKey || e.altKey) { return; }

    switch (e.key) {
      case 'j': case 'ArrowDown':  e.preventDefault(); move(1); break;
      case 'k': case 'ArrowUp':    e.preventDefault(); move(-1); break;
      case 'g': e.preventDefault(); sel = 0; mark(); break;
      case 'G': e.preventDefault(); sel = visible().length - 1; mark(); break;
      case 'c': case 's': case 'r': case 'd': case 'u':
        e.preventDefault(); act(e.key); break;
      case 'n': e.preventDefault(); document.getElementById('new-title').focus(); break;
      case '/': e.preventDefault(); filter.focus(); break;
      case 'Enter': e.preventDefault(); toggleDetail(); break;
      case '?':
        e.preventDefault();
        showHelp(help.hidden);
        break;
      default: break;
    }
  });

  helpOn.addEventListener('click', function () { showHelp(help.hidden); });

  host.addEventListener('click', function (e) {
    var row = e.target.closest('.row');
    if (!row || e.target.closest('.acts')) { return; }
    var shown = visible();
    sel = shown.indexOf(row);
    mark();
    toggleDetail();
  });

  filter.addEventListener('input', mark);

  /* Re-render only on real change, and carry selection + open detail across it. */
  function adopt(html) {
    var keepSel = current();
    var keepId  = keepSel ? keepSel.dataset.task : null;
    var openId  = null;
    var open = host.querySelector('.detail:not([hidden])');
    if (open) { openId = open.parentElement.dataset.task; }

    host.innerHTML = html;
    sel = 0;
    if (keepId) {
      var shown = visible();
      var i = shown.findIndex(function (r) { return r.dataset.task === keepId; });
      if (i > -1) { sel = i; }
    }
    if (openId) {
      var target = host.querySelector('[data-task="' + openId + '"] .detail');
      if (target) { target.hidden = false; }
    }
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
