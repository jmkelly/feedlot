// Feedlot — Custom JS enhancements
// Scroll-past-mark-as-read, keyboard shortcuts, scroll progress, and UX polish.
// Uses HTMX OOB (out-of-band) swaps for sidebar badge updates — the server
// is the single source of truth for unread counts.

(function() {
  'use strict';

  // ─── State ──────────────────────────────────────────────────────────

  const seenIds = new Set();
  let scrollObserver = null;
  let scrollMarkReadEnabled = localStorage.getItem('feedlot:scrollMarkRead') === 'true';
  let focusedArticleIndex = -1;
  let progressTicking = false;

  // ─── Utility ────────────────────────────────────────────────────────

  /** Extract article ID from the card's toggle button hx-post attribute */
  function getArticleId(card) {
    var btn = card.querySelector('button[hx-post*="/articles/"][hx-post*="/toggle"]');
    if (!btn) return null;
    var match = btn.getAttribute('hx-post').match(/\/articles\/(\d+)\/toggle/);
    return match ? match[1] : null;
  }

  /** Check whether a card is currently unread */
  function isUnreadCard(card) {
    var btn = card.querySelector('button[hx-post*="/toggle"]');
    if (!btn) return false;
    return btn.getAttribute('title') === 'Mark read' || btn.textContent.trim() === '\u25CF';
  }

  /** Mark a single card as read: POST, update classes, trigger sidebar refresh */
  function markCardRead(card) {
    var id = getArticleId(card);
    if (!id || seenIds.has(id)) return;
    seenIds.add(id);

    // POST to backend (MarkRead handler returns 204, no OOB needed)
    fetch('/articles/' + id + '/read', { method: 'POST' })['catch'](function(err) {
      console.error('Failed to mark article', id, 'read:', err);
    });

    // Update card UI
    card.classList.add('is-read');
    card.classList.remove('is-unread');
    card.classList.add('is-marked');
    setTimeout(function() {
      card.classList.remove('is-marked');
    }, 800);
  }

  /** Find an article card by its ID */
  function findCardById(id) {
    var cards = document.querySelectorAll('.card[data-article-id]');
    for (var i = 0; i < cards.length; i++) {
      if (getArticleId(cards[i]) === id) return cards[i];
    }
    return null;
  }

  // ─── Sidebar refresh (for non-HTMX operations) ─────────────────────

  /** Refresh the feed sidebar from the server for authoritative unread counts */
  function refreshFeedSidebar() {
    if (typeof htmx !== 'undefined' && htmx.ajax) {
      htmx.ajax('GET', '/feeds', { target: '#feed-sidebar-inner', swap: 'innerHTML' });
    }
  }

  // Debounced sidebar refresh for bulk/fetch-based operations
  var _sidebarRefreshTimer = null;
  function debouncedRefreshSidebar() {
    if (_sidebarRefreshTimer) clearTimeout(_sidebarRefreshTimer);
    _sidebarRefreshTimer = setTimeout(function() {
      refreshFeedSidebar();
      _sidebarRefreshTimer = null;
    }, 400);
  }

  // ─── IntersectionObserver: scroll-past-mark-as-read ────────────────

  function setupScrollObserver() {
    if (scrollObserver) {
      scrollObserver.disconnect();
      scrollObserver = null;
    }
    if (!scrollMarkReadEnabled) return;

    scrollObserver = new IntersectionObserver(function(entries) {
      entries.forEach(function(entry) {
        var card = entry.target;
        if (!isUnreadCard(card)) return;
        var id = getArticleId(card);
        if (!id || seenIds.has(id)) return;

        if (entry.isIntersecting) {
          // Card is visible — start a 500ms timer
          if (!card._readTimer) {
            card._readTimer = setTimeout(function() {
              if (document.contains(card) && isUnreadCard(card) && !seenIds.has(getArticleId(card))) {
                markCardRead(card);
                debouncedRefreshSidebar();
              }
              card._readTimer = null;
            }, 500);
          }
        } else {
          // Card left the viewport
          if (card._readTimer) {
            clearTimeout(card._readTimer);
            card._readTimer = null;
          }
          // If top edge is above the viewport, mark immediately
          if (entry.boundingClientRect.top < 0) {
            markCardRead(card);
            debouncedRefreshSidebar();
          }
        }
      });
    }, {
      root: null,
      rootMargin: '0px',
      threshold: 0
    });

    // Observe all article cards
    document.querySelectorAll('.card[data-article-id]').forEach(function(card) {
      scrollObserver.observe(card);
    });
  }

  /** Re-scan the DOM and observe any new cards after HTMX swaps */
  function rescanCards() {
    if (scrollObserver) {
      document.querySelectorAll('.card[data-article-id]').forEach(function(card) {
        scrollObserver.observe(card);
      });
    }
  }

  // ─── Scroll‑read toggle chip ───────────────────────────────────────

  function handleScrollReadToggle() {
    var toggle = document.getElementById('scroll-read-toggle');
    if (!toggle) return;
    var isPressed = toggle.getAttribute('aria-pressed') === 'true';
    var newState = !isPressed;
    toggle.setAttribute('aria-pressed', String(newState));
    scrollMarkReadEnabled = newState;
    localStorage.setItem('feedlot:scrollMarkRead', String(newState));
    if (newState) {
      setupScrollObserver();
    } else {
      if (scrollObserver) {
        scrollObserver.disconnect();
        scrollObserver = null;
      }
    }
  }

  // ─── Scroll progress bar ───────────────────────────────────────────

  function updateProgress() {
    if (progressTicking) return;
    progressTicking = true;
    window.requestAnimationFrame(function() {
      var progressBar = document.getElementById('progress-bar');
      if (!progressBar) {
        progressTicking = false;
        return;
      }

      var articleList = document.getElementById('article-list');
      var scrollTop, scrollHeight, clientHeight;

      if (articleList) {
        scrollTop = articleList.scrollTop;
        scrollHeight = articleList.scrollHeight;
        clientHeight = articleList.clientHeight;
      } else {
        scrollTop = window.scrollY || document.documentElement.scrollTop;
        scrollHeight = document.documentElement.scrollHeight;
        clientHeight = window.innerHeight;
      }

      var maxScroll = Math.max(scrollHeight - clientHeight, 1);
      var percent = Math.min((scrollTop / maxScroll) * 100, 100);
      progressBar.style.width = percent + '%';
      progressTicking = false;
    });
  }

  // ─── Mark all read ─────────────────────────────────────────────────

  /** Collect IDs from all unread cards and POST sequentially */
  function handleMarkAllRead() {
    var cards = document.querySelectorAll('.card[data-article-id]');
    var ids = [];
    cards.forEach(function(card) {
      if (isUnreadCard(card)) {
        var id = getArticleId(card);
        if (id) ids.push(id);
      }
    });

    if (ids.length === 0) return;

    function postNext(i) {
      if (i >= ids.length) {
        // All done — refresh sidebar from server for authoritative counts
        refreshFeedSidebar();
        return;
      }
      fetch('/articles/' + ids[i] + '/read', { method: 'POST' })
        .then(function() {
          var card = findCardById(ids[i]);
          if (card) {
            card.classList.add('is-read');
            card.classList.remove('is-unread');
            card.classList.add('is-marked');
            setTimeout(function() { card.classList.remove('is-marked'); }, 800);
          }
          postNext(i + 1);
        })
        ['catch'](function(err) {
          console.error('Failed to mark article', ids[i], 'read:', err);
          postNext(i + 1);
        });
    }
    postNext(0);
  }

  // ─── Keyboard Shortcuts ────────────────────────────────────────────

  function getArticleCards() {
    return document.querySelectorAll('.card[data-article-id]');
  }

  function navigateArticle(direction) {
    var articles = getArticleCards();
    if (articles.length === 0) return;

    // Remove previous focus
    if (focusedArticleIndex >= 0 && focusedArticleIndex < articles.length) {
      articles[focusedArticleIndex].classList.remove('is-focused');
    }

    focusedArticleIndex += direction;
    if (focusedArticleIndex < 0) focusedArticleIndex = 0;
    if (focusedArticleIndex >= articles.length) focusedArticleIndex = articles.length - 1;

    articles[focusedArticleIndex].classList.add('is-focused');
    articles[focusedArticleIndex].scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }

  function toggleFocusedRead() {
    var articles = getArticleCards();
    if (focusedArticleIndex < 0 || focusedArticleIndex >= articles.length) {
      // Find the first unread article
      for (var i = 0; i < articles.length; i++) {
        var btn = articles[i].querySelector('button[hx-post*="/toggle"]');
        if (btn) {
          btn.click();
          return;
        }
      }
      return;
    }
    var btn = articles[focusedArticleIndex].querySelector('button[hx-post*="/toggle"]');
    if (btn) btn.click();
  }

  function focusAddFeed() {
    var input = document.querySelector('input[name="url"]');
    if (input) {
      input.focus();
      input.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  }

  function navigateFeed(direction) {
    var feeds = document.querySelectorAll('.feed[data-feed-id]');
    if (feeds.length === 0) return;

    // Find the currently active feed index
    var currentIndex = -1;
    feeds.forEach(function(feed, i) {
      if (feed.classList.contains('feed--active')) {
        currentIndex = i;
      }
    });

    var targetIndex;
    if (currentIndex === -1) {
      targetIndex = direction > 0 ? 0 : feeds.length - 1;
    } else {
      targetIndex = currentIndex + direction;
    }

    if (targetIndex < 0 || targetIndex >= feeds.length) return;

    var link = feeds[targetIndex].querySelector('a[hx-get]');
    if (link) link.click();
  }

  // ─── Theme toggle ────────────────────────────────────────────────

  function currentTheme() {
    return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
  }

  function setTheme(theme, persist) {
    document.documentElement.setAttribute('data-theme', theme);
    if (persist) {
      try { localStorage.setItem('feedlot:theme', theme); } catch (e) {}
    }
  }

  function handleThemeToggle() {
    setTheme(currentTheme() === 'dark' ? 'light' : 'dark', true);
  }

  // Follow the system theme unless the user has made an explicit choice
  var systemDark = window.matchMedia('(prefers-color-scheme: dark)');
  systemDark.addEventListener('change', function(e) {
    var stored = null;
    try { stored = localStorage.getItem('feedlot:theme'); } catch (err) {}
    if (stored !== 'light' && stored !== 'dark') {
      setTheme(e.matches ? 'dark' : 'light', false);
    }
  });

  // ─── Wiring ─────────────────────────────────────────────────────
  // Note: login/register submit through HTMX and swap the whole <body>,
  // so dashboard controls may appear long after DOMContentLoaded. Wire
  // element-specific state on both events, and use delegated clicks for
  // buttons so handlers survive body swaps.

  function wireControls() {
    // Restore persisted scroll-read state onto the chip, if present
    var scrollToggle = document.getElementById('scroll-read-toggle');
    if (scrollToggle) {
      scrollToggle.setAttribute('aria-pressed', String(scrollMarkReadEnabled));
    }

    // Scroll progress bar
    var progressBar = document.getElementById('progress-bar');
    if (progressBar && !progressBar._wired) {
      progressBar._wired = true;
      var articleList = document.getElementById('article-list');
      var scrollContainer = articleList || window;
      scrollContainer.addEventListener('scroll', updateProgress);
      window.addEventListener('resize', updateProgress);
    }
    if (progressBar) {
      updateProgress();
    }

    // Scroll-mark-read observer
    if (scrollMarkReadEnabled) {
      setupScrollObserver();
    }

    // Auto-dismiss flash messages after 5 seconds
    var flashMessages = document.querySelectorAll('.alert:not(._timed)');
    flashMessages.forEach(function(msg) {
      if (msg.textContent.trim() !== '') {
        msg._timed = true;
        setTimeout(function() {
          msg.style.transition = 'opacity 300ms ease-out';
          msg.style.opacity = '0';
          setTimeout(function() {
            msg.style.display = 'none';
          }, 300);
        }, 5000);
      }
    });
  }

  document.addEventListener('DOMContentLoaded', wireControls);

  // Delegated control clicks — survive HTMX body swaps
  document.addEventListener('click', function(e) {
    var t = e.target;
    if (!(t instanceof Element)) return;
    if (t.closest('#theme-toggle')) { handleThemeToggle(); return; }
    if (t.closest('#scroll-read-toggle')) { handleScrollReadToggle(); return; }
    if (t.closest('#mark-all-read')) { handleMarkAllRead(); return; }
  });

  // ─── Global keyboard shortcuts ─────────────────────────────────────

  document.addEventListener('keydown', function(e) {
    // Don't capture when typing in inputs
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
      return;
    }

    switch (e.key) {
      case 'j':
      case 'J':
        e.preventDefault();
        navigateArticle(1);
        break;
      case 'k':
      case 'K':
        e.preventDefault();
        navigateArticle(-1);
        break;
      case 'r':
      case 'R':
        e.preventDefault();
        toggleFocusedRead();
        break;
      case 'f':
      case 'F':
        e.preventDefault();
        focusAddFeed();
        break;
      case 'n':
      case 'N':
        e.preventDefault();
        navigateFeed(1);
        break;
      case 'p':
      case 'P':
        e.preventDefault();
        navigateFeed(-1);
        break;
    }
  });

  // ─── HTMX Event Handlers ───────────────────────────────────────────

  // After any HTMX swap, reset state and re-wire controls/cards.
  // Sidebar badge updates are handled by the server via hx-swap-oob
  // in the ToggleRead response — no client-side arithmetic needed.
  document.addEventListener('htmx:afterSwap', function(e) {
    focusedArticleIndex = -1;
    seenIds.clear();
    wireControls();
    rescanCards();
  });

  document.addEventListener('htmx:afterRequest', function(e) {
    var trigger = e.detail.elt;
    if (trigger && trigger.hasAttribute('hx-post') && trigger.getAttribute('hx-post').indexOf('/refresh') !== -1) {
      trigger.textContent = '↻';
      trigger.style.animation = '';
    }
  });

})();
