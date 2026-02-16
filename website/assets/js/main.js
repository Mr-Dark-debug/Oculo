document.addEventListener('DOMContentLoaded', () => {
    initSmoothScroll();
    initNavbarScroll();
    initMagneticButtons();
    initScrollReveal();
    loadComponents();
});

function initNavbarScroll() {
    const nav = document.querySelector('.nav-premium');
    if (!nav) return;
    
    window.addEventListener('scroll', () => {
        if (window.scrollY > 50) {
            nav.classList.add('scrolled');
        } else {
            nav.classList.remove('scrolled');
        }
    });
}

function initMagneticButtons() {
    const buttons = document.querySelectorAll('.btn-premium');
    
    buttons.forEach(btn => {
        btn.addEventListener('mousemove', (e) => {
            const rect = btn.getBoundingClientRect();
            const x = e.clientX - rect.left - rect.width / 2;
            const y = e.clientY - rect.top - rect.height / 2;
            
            btn.style.transform = `translate(${x * 0.2}px, ${y * 0.3}px)`;
        });
        
        btn.addEventListener('mouseleave', () => {
            btn.style.transform = 'translate(0, 0)';
        });
    });
}

function initScrollReveal() {
    const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.classList.add('active');
            }
        });
    }, { threshold: 0.1 });

    document.querySelectorAll('.reveal').forEach(el => observer.observe(el));
}

function initSmoothScroll() {
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
        anchor.addEventListener('click', function (e) {
            e.preventDefault();
            const targetId = this.getAttribute('href');
            if (targetId === '#') return;
            
            const target = document.querySelector(targetId);
            if (target) {
                window.scrollTo({
                    top: target.offsetTop - 100,
                    behavior: 'smooth'
                });
            }
        });
    });
}

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
                
                // Adjustment for subpages
                if (isSubPage) {
                    html = html.replace(/website\//g, '');
                    html = html.replace(/index\.html/g, '../index.html');
                    html = html.replace(/banner\.svg/g, '../banner.svg');
                }
                
                el.innerHTML = html;
                
                // Component-specific inits
                if (name === 'hero') initHeroTyping();
                if (name === 'github-stats') fetchGitHubStats();
                if (name === 'contributors') fetchContributors();
            }
        } catch (e) {
            console.error(`Error loading component: ${name}`, e);
        }
    }
}

function initHeroTyping() {
    const terminal = document.getElementById('hero-code');
    if (!terminal) return;
    
    const code = [
        '<span style="color:#818cf8">oculo</span> trace --agent researcher',
        '<span style="color:#94a3b8"># Ingesting memory mutation...</span>',
        '<span style="color:#10b981">Diff captured:</span> <span style="color:#f59e0b">"context_window"</span> changed.',
        '<span style="color:#818cf8">oculo</span> analyze report --format=tui'
    ];
    
    let i = 0;
    terminal.innerHTML = '';
    
    function type() {
        if (i < code.length) {
            const p = document.createElement('div');
            p.style.marginBottom = '8px';
            p.innerHTML = `<span style="color:#475569; margin-right:12px">â€º</span> ${code[i]}`;
            p.style.opacity = '0';
            p.style.transform = 'translateX(-10px)';
            p.style.transition = 'all 0.5s ease';
            terminal.appendChild(p);
            
            setTimeout(() => {
                p.style.opacity = '1';
                p.style.transform = 'translateX(0)';
                i++;
                setTimeout(type, 1000);
            }, 50);
        }
    }
    
    type();
}

async function fetchGitHubStats() {
    const repo = 'Mr-Dark-debug/Oculo';
    try {
        const res = await fetch(`https://api.github.com/repos/${repo}`);
        if (res.ok) {
            const data = await res.json();
            const s = document.getElementById('gh-stars-val');
            const f = document.getElementById('gh-forks-val');
            if (s) s.innerText = data.stargazers_count;
            if (f) f.innerText = data.forks_count;
        }
    } catch (e) {}
}

async function fetchContributors() {
    const repo = 'Mr-Dark-debug/Oculo';
    try {
        const res = await fetch(`https://api.github.com/repos/${repo}/contributors`);
        if (res.ok) {
            const users = await res.json();
            const container = document.getElementById('contributors-scroll');
            if (container) {
                container.innerHTML = users.map(u => `
                    <a href="${u.html_url}" target="_blank" class="avatar-link" title="${u.login}">
                        <img src="${u.avatar_url}" alt="${u.login}">
                    </a>
                `).join('');
            }
        }
    } catch (e) {}
}
