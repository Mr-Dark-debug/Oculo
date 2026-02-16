document.addEventListener('DOMContentLoaded', () => {
    initSmoothScroll();
    initNavbarScroll();
    initScrollReveal();
    loadComponents();
    initHeroTyping();
    fetchGitHubStats();
    fetchContributors();
    initFullContributors();
});

/* ─── Navbar ──────────────────────────── */
function initNavbarScroll() {
    const nav = document.querySelector('.nav-premium');
    if (!nav) return;

    window.addEventListener('scroll', () => {
        nav.classList.toggle('scrolled', window.scrollY > 50);
    }, { passive: true });
}

/* ─── Mobile Menu ─────────────────────── */
window.toggleMobileMenu = function () {
    const menu = document.querySelector('.mobile-menu');
    const toggle = document.querySelector('.nav-toggle');
    const body = document.body;

    if (menu && toggle) {
        menu.classList.toggle('active');
        toggle.classList.toggle('open');

        // Prevent scrolling when menu is open
        if (menu.classList.contains('active')) {
            body.style.overflow = 'hidden';
        } else {
            body.style.overflow = '';
        }
    }
}

function highlightActiveLink() {
    const currentPath = window.location.pathname;
    const links = document.querySelectorAll('.nav-link-premium');

    links.forEach(link => {
        const href = link.getAttribute('href');
        if (!href) return;

        // Clean paths for comparison
        const linkPath = href.replace('../', '').replace('./', '');
        const pageName = currentPath.split('/').pop() || 'index.html';

        // Strict match for index, partial for others
        if (linkPath === pageName || (pageName === '' && linkPath === 'index.html')) {
            link.classList.add('active');
        } else if (linkPath.endsWith(pageName) && pageName !== 'index.html') {
            link.classList.add('active');
        }
    });
}

/* ─── Scroll Reveal ───────────────────── */
function initScrollReveal() {
    const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.classList.add('active');
            }
        });
    }, { threshold: 0.08, rootMargin: '0px 0px -40px 0px' });

    document.querySelectorAll('.reveal').forEach(el => observer.observe(el));
}

/* ─── Smooth Scroll ───────────────────── */
function initSmoothScroll() {
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
        anchor.addEventListener('click', function (e) {
            e.preventDefault();
            const targetId = this.getAttribute('href');
            if (targetId === '#') return;
            const target = document.querySelector(targetId);
            if (target) {
                window.scrollTo({ top: target.offsetTop - 80, behavior: 'smooth' });

                // Close mobile menu if open
                if (document.querySelector('.mobile-menu.active')) {
                    toggleMobileMenu();
                }
            }
        });
    });
}

/* ─── Component Loader ────────────────── */
async function loadComponents() {
    const isSubPage = window.location.pathname.includes('/website/');
    const basePath = isSubPage ? 'components/' : 'website/components/';

    const components = document.querySelectorAll('[data-component]');

    for (const el of components) {
        const name = el.getAttribute('data-component');
        try {
            const response = await fetch(`${basePath}${name}.html`);
            if (response.ok) {
                let html = await response.text();

                if (isSubPage) {
                    html = html.replace(/website\//g, '');
                    html = html.replace(/index\.html/g, '../index.html');
                    html = html.replace(/banner\.svg/g, '../banner.svg');
                }

                el.innerHTML = html;

                // Component-specific inits
                if (name === 'navbar') {
                    highlightActiveLink();
                    // Re-bind click listeners for mobile menu links to close menu
                    el.querySelectorAll('.mobile-menu a').forEach(a => {
                        a.addEventListener('click', toggleMobileMenu);
                    });
                }
                if (name === 'hero') initHeroTyping();
                if (name === 'github-stats') fetchGitHubStats();

                // Re-observe newly loaded elements
                el.querySelectorAll('.reveal').forEach(r => {
                    const observer = new IntersectionObserver((entries) => {
                        entries.forEach(entry => {
                            if (entry.isIntersecting) entry.target.classList.add('active');
                        });
                    }, { threshold: 0.08, rootMargin: '0px 0px -40px 0px' });
                    observer.observe(r);
                });
            }
        } catch (e) {
            console.error(`Error loading component: ${name}`, e);
        }
    }
}

/* ─── Hero Typing Effect ──────────────── */
function initHeroTyping() {
    const terminal = document.getElementById('hero-code');
    if (!terminal) return;

    const code = [
        '<span style="color:#818cf8">$</span> oculo trace --agent researcher-v1',
        '<span style="color:#64748b"># Ingesting memory mutations...</span>',
        '<span style="color:#10b981">✓</span> Diff captured: <span style="color:#f59e0b">"context_window"</span> updated',
        '<span style="color:#10b981">✓</span> Token usage: <span style="color:#a5f3fc">1,247</span> prompt / <span style="color:#a5f3fc">892</span> completion',
        '<span style="color:#818cf8">$</span> oculo tui',
        '<span style="color:#10b981">●</span> TUI launched on trace <span style="color:#f59e0b">t_8f3a</span>'
    ];

    let i = 0;
    terminal.innerHTML = '';

    function type() {
        if (i < code.length) {
            const p = document.createElement('div');
            p.style.marginBottom = '6px';
            p.innerHTML = code[i];
            p.style.opacity = '0';
            p.style.transform = 'translateX(-8px)';
            p.style.transition = 'all 0.4s ease';
            terminal.appendChild(p);

            setTimeout(() => {
                p.style.opacity = '1';
                p.style.transform = 'translateX(0)';
                i++;
                setTimeout(type, 800);
            }, 50);
        } else {
            // Blinking cursor
            const cursor = document.createElement('div');
            cursor.innerHTML = '<span style="color:#818cf8">$</span> <span style="animation: blink 1s step-end infinite; color:#94a3b8;">▋</span>';
            terminal.appendChild(cursor);
        }
    }

    type();
}

/* ─── GitHub Stats ────────────────────── */
async function fetchGitHubStats() {
    const repo = 'Mr-Dark-debug/Oculo';
    try {
        const res = await fetch(`https://api.github.com/repos/${repo}`);
        if (res.ok) {
            const data = await res.json();
            const s = document.getElementById('gh-stars-val');
            const f = document.getElementById('gh-forks-val');
            const iss = document.getElementById('gh-issues-val');
            if (s) s.innerText = data.stargazers_count;
            if (f) f.innerText = data.forks_count;
            if (iss) iss.innerText = data.open_issues_count;
        }
    } catch (e) { }
}

/* ─── Homepage Contributors ───────────── */
async function fetchContributors() {
    const container = document.getElementById('contributors-scroll');
    if (!container) return;

    const repo = 'Mr-Dark-debug/Oculo';
    try {
        const res = await fetch(`https://api.github.com/repos/${repo}/contributors`);
        if (res.ok) {
            const users = await res.json();
            container.innerHTML = users.slice(0, 12).map(u => `
                <a href="${u.html_url}" target="_blank" class="avatar-link" title="${u.login}" style="text-decoration:none;">
                    <img src="${u.avatar_url}" alt="${u.login}">
                </a>
            `).join('');
        }
    } catch (e) {
        container.innerHTML = '<p style="color: var(--text-faint);">Could not load contributors.</p>';
    }
}

/* ─── Full Contributors Page ──────────── */
async function initFullContributors() {
    const container = document.getElementById('full-contributors-list');
    if (!container) return;

    const repo = 'Mr-Dark-debug/Oculo';
    try {
        const res = await fetch(`https://api.github.com/repos/${repo}/contributors`);
        if (res.ok) {
            const users = await res.json();
            container.innerHTML = users.map(u => `
                <div class="contributor-card reveal">
                    <img src="${u.avatar_url}" alt="${u.login}">
                    <h4>${u.login}</h4>
                    <p>${u.contributions} contributions</p>
                    <a href="${u.html_url}" target="_blank">View Profile →</a>
                </div>
            `).join('');

            // Observe newly added reveal elements
            container.querySelectorAll('.reveal').forEach(el => {
                const observer = new IntersectionObserver((entries) => {
                    entries.forEach(entry => {
                        if (entry.isIntersecting) entry.target.classList.add('active');
                    });
                }, { threshold: 0.08 });
                observer.observe(el);
            });
        }
    } catch (e) {
        container.innerHTML = '<div style="grid-column: 1 / -1; text-align: center; padding: 4rem; color: var(--text-muted);">Could not load contributors. Please check back later.</div>';
    }
}
