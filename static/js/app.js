// Feedlot — Custom JS enhancements
// HTMX event handlers, keyboard shortcuts, and UX polish.

(function() {
  'use strict';

  // ─── Keyboard Shortcuts ────────────────────────────────────────────

  document.addEventListener('keydown', function(e) {
    // Don't capture when typing in inputs
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
      return;
    }

    switch (e.key) {
      case 'j':
      case 'J':
        // Next article: focus next article card
        e.preventDefault();
        navigateArticle(1);
        break;
      case 'k':
      case 'K':
        // Previous article: focus previous article card
        e.preventDefault();
        navigateArticle(-1);
        break;
      case 'r':
      case 'R':
        // Toggle read on the first visible unread article
        e.preventDefault();
        toggleFocusedRead();
        break;
      case 'f':
      case 'F':
        // Focus the add-feed input
        e.preventDefault();
        focusAddFeed();
        break;
      case 'n':
      case 'N':
        // Focus the next feed in sidebar
        e.preventDefault();
        navigateFeed(1);
        break;
      case 'p':
      case 'P':
        // Focus the previous feed in sidebar
        e.preventDefault();
        navigateFeed(-1);
        break;
    }
  });

  let focusedArticleIndex = -1;

  function getArticleCards() {
    return document.querySelectorAll('.article-card');
  }

  function navigateArticle(direction) {
    const articles = getArticleCards();
    if (articles.length === 0) return;

    // Remove previous focus
    if (focusedArticleIndex >= 0 && focusedArticleIndex < articles.length) {
      articles[focusedArticleIndex].style.outline = '';
    }

    focusedArticleIndex += direction;

    // Clamp
    if (focusedArticleIndex < 0) focusedArticleIndex = 0;
    if (focusedArticleIndex >= articles.length) focusedArticleIndex = articles.length - 1;

    // Add focus style
    articles[focusedArticleIndex].style.outline = '2px solid #d97706';
    articles[focusedArticleIndex].style.outlineOffset = '2px';

    // Scroll into view
    articles[focusedArticleIndex].scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }

  function toggleFocusedRead() {
    const articles = getArticleCards();
    if (focusedArticleIndex < 0 || focusedArticleIndex >= articles.length) {
      // Find the first unread article
      for (let i = 0; i < articles.length; i++) {
        const btn = articles[i].querySelector('button[hx-post*="/toggle"]');
        if (btn) {
          btn.click();
          return;
        }
      }
      return;
    }

    const btn = articles[focusedArticleIndex].querySelector('button[hx-post*="/toggle"]');
    if (btn) {
      btn.click();
    }
  }

  function focusAddFeed() {
    const input = document.querySelector('input[name="url"]');
    if (input) {
      input.focus();
      input.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  }

  function navigateFeed(direction) {
    const feeds = document.querySelectorAll('.feed-item');
    if (feeds.length === 0) return;

    // Find current active feed index
    let currentIndex = -1;
    feeds.forEach((feed, i) => {
      if (feed.classList.contains('bg-amber-50')) {
        currentIndex = i;
      }
    });

    let targetIndex;
    if (currentIndex === -1) {
      targetIndex = direction > 0 ? 0 : feeds.length - 1;
    } else {
      targetIndex = currentIndex + direction;
    }

    if (targetIndex < 0 || targetIndex >= feeds.length) return;

    const link = feeds[targetIndex].querySelector('a[hx-get]');
    if (link) {
      link.click();
    }
  }

  // ─── HTMX Event Handlers ───────────────────────────────────────────

  // After any HTMX swap, re-run any needed state
  document.addEventListener('htmx:afterSwap', function(e) {
    // Reset focused article index after list refresh
    focusedArticleIndex = -1;
  });

  // Show loading state during feed refresh
  document.addEventListener('htmx:beforeRequest', function(e) {
    const trigger = e.detail.elt;
    if (trigger && trigger.hasAttribute('hx-post') && trigger.getAttribute('hx-post').includes('/refresh')) {
      trigger.textContent = '⟳';
      trigger.style.animation = 'spin 1s linear infinite';
    }
  });

  document.addEventListener('htmx:afterRequest', function(e) {
    const trigger = e.detail.elt;
    if (trigger && trigger.hasAttribute('hx-post') && trigger.getAttribute('hx-post').includes('/refresh')) {
      trigger.textContent = '↻';
      trigger.style.animation = '';
    }
  });

  // ─── Add spin animation ────────────────────────────────────────────

  const style = document.createElement('style');
  style.textContent = `
    @keyframes spin {
      from { transform: rotate(0deg); }
      to { transform: rotate(360deg); }
    }
  `;
  document.head.appendChild(style);

  // ─── Auto-refresh indicator ────────────────────────────────────────

  // If there's a flash message, auto-dismiss it after 5 seconds
  document.addEventListener('DOMContentLoaded', function() {
    const flashMessages = document.querySelectorAll('[class*="bg-red-50"], [class*="bg-green-50"]');
    flashMessages.forEach(function(msg) {
      if (msg.textContent.trim() !== '') {
        setTimeout(function() {
          msg.style.transition = 'opacity 300ms ease-out';
          msg.style.opacity = '0';
          setTimeout(function() {
            msg.style.display = 'none';
          }, 300);
        }, 5000);
      }
    });
  });

})();
