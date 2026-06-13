// Vior marketing site — parallax + scroll-reveal interactions.
// Pure DOM, no deps. Respects prefers-reduced-motion.
(function () {
  'use strict';

  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  // ── 1. Scroll reveal via IntersectionObserver ──────────────────
  // Anything with .reveal or .reveal-stagger gets `.in-view` added
  // when ~30% visible, triggering the CSS fade+rise.
  const io = new IntersectionObserver(function (entries) {
    entries.forEach(function (e) {
      if (e.isIntersecting) {
        e.target.classList.add('in-view');
        io.unobserve(e.target);
      }
    });
  }, { threshold: 0.18, rootMargin: '0px 0px -10% 0px' });

  document.querySelectorAll('.reveal, .reveal-stagger').forEach(function (el) {
    io.observe(el);
  });

  // ── 2. Hero radar tilt — follows cursor in real time ────────────
  const root = document.documentElement;
  let raf = 0, tx = 0, ty = 0;
  function setMouse(x, y) {
    tx = (x / window.innerWidth) * 2 - 1;   // -1 → 1
    ty = (y / window.innerHeight) * 2 - 1;
    if (raf) return;
    raf = requestAnimationFrame(function () {
      root.style.setProperty('--mx', tx.toFixed(3));
      root.style.setProperty('--my', ty.toFixed(3));
      raf = 0;
    });
  }
  window.addEventListener('mousemove', function (e) { setMouse(e.clientX, e.clientY); }, { passive: true });

  // Touch fallback — sample first touch.
  window.addEventListener('touchmove', function (e) {
    if (e.touches[0]) setMouse(e.touches[0].clientX, e.touches[0].clientY);
  }, { passive: true });

  // ── 3. Sticky nav shadow / blur once scrolled past hero ─────────
  const nav = document.querySelector('.nav');
  let lastScroll = -1;
  function onScroll() {
    const y = window.scrollY;
    if (y === lastScroll) return;
    lastScroll = y;
    if (nav) nav.classList.toggle('scrolled', y > 40);

    // 4. Drift the radial halo inside each .section as it scrolls past.
    document.querySelectorAll('.section').forEach(function (section) {
      const rect = section.getBoundingClientRect();
      const visible = Math.max(0, Math.min(window.innerHeight, rect.bottom)
                       - Math.max(0, rect.top));
      if (visible <= 0) return;
      // Translate the ::before halo as the section scrolls.
      const progress = 1 - Math.max(0, rect.top) / window.innerHeight;
      section.style.setProperty('--py', (progress * 80 - 40).toFixed(0) + 'px');
    });
  }
  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();

  // ── 5. Magnetic feature cards — slight pull toward cursor ───────
  document.querySelectorAll('.feature').forEach(function (card) {
    card.addEventListener('mousemove', function (e) {
      const r = card.getBoundingClientRect();
      const px = (e.clientX - r.left) / r.width  - 0.5;
      const py = (e.clientY - r.top)  / r.height - 0.5;
      card.style.transform =
        'translate(' + (px * 6).toFixed(1) + 'px,' + (py * 6 - 5).toFixed(1) + 'px) ' +
        'rotateX(' + (-py * 4).toFixed(2) + 'deg) ' +
        'rotateY(' + ( px * 4).toFixed(2) + 'deg) ' +
        'scale(1.015)';
    });
    card.addEventListener('mouseleave', function () { card.style.transform = ''; });
  });
})();

// ── 6. Live release tag from GitHub API ─────────────────────────
// Replaces the hard-coded "v0.4 · phase 2 shipped" pill + Download
// button href with whatever the latest GitHub release actually is.
// Public REST API → 60 req/hr per IP, no auth needed.
(async function fetchLatestRelease() {
  const tagEl = document.getElementById('release-tag');
  const ctaEl = document.getElementById('release-cta');
  if (!tagEl && !ctaEl) return;

  try {
    const res = await fetch('https://api.github.com/repos/subhashraveendran/Vior/releases/latest', {
      headers: { 'Accept': 'application/vnd.github+json' },
    });
    if (!res.ok) return; // 404 = no releases yet; leave the hardcoded fallback
    const r = await res.json();

    const tag    = r.tag_name || 'latest';
    const name   = (r.name && r.name !== r.tag_name) ? r.name : '';
    const date   = r.published_at ? new Date(r.published_at).toLocaleDateString(
                       undefined, { month: 'short', day: 'numeric', year: 'numeric' }) : '';
    const dlAsset = (r.assets || []).find(a =>
      /\.(dmg|exe|zip|tar\.gz|appimage|apk)$/i.test(a.name)
    );

    if (tagEl) {
      const parts = [tag];
      if (name)  parts.push(name);
      else if (date) parts.push(date);
      tagEl.textContent = parts.join(' · ');
    }
    if (ctaEl && dlAsset && dlAsset.browser_download_url) {
      ctaEl.href = dlAsset.browser_download_url;
    } else if (ctaEl) {
      ctaEl.href = r.html_url; // release page
    }
  } catch (e) {
    // Network blocked? leave fallback. console.warn for devs.
    console.warn('[vior] release fetch failed:', e);
  }
})();
