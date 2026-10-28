// Lyrics follow the authoritative room/audio position at 50ms precision.
import { state, on, playbackPosition, lyricsAPI, canControl, send } from './core.js?v=88';

export function normalizeLyrics(data) {
    if (!data || data.unsupported) return { status: 'unsupported', lines: [] };
    const lrc = typeof data.lrc === 'string' ? data.lrc : '';
    const timestamp = /\[(\d+):(\d{2})(?:[.:](\d{1,3}))?\]/g;
    const parsedOffset = Number(lrc.match(/\[offset:([+-]?\d+)\]/i)?.[1] || 0) / 1000;
    const offset = Number.isFinite(parsedOffset) ? parsedOffset : 0;
    const lines = lrc.split(/\r?\n/).flatMap(row => {
        const text = row.replace(timestamp, '').trim();
        return [...row.matchAll(timestamp)].filter(match => Number(match[2]) < 60).map(match => {
            const frac = (match[3] || '0').padEnd(3, '0').slice(0, 3);
            return {
                time: Math.max(0, Number(match[1]) * 60 + Number(match[2]) + Number(frac) / 1000 + offset),
                text
            };
        }).filter(line => Number.isFinite(line.time));
    });
    if (lines.some(line => line.text.trim())) return { status: 'synced', lines: lines.sort((a, b) => a.time - b.time) };
    const plain = typeof data.text === 'string' ? data.text.trim() : '';
    if (plain) {
        const rawLines = plain.split(/\r?\n/).map(t => t.trim()).filter(Boolean);
        const dur = Math.max(30, Number(state.room?.track?.duration) || rawLines.length * 4);
        const step = Math.max(1.5, (dur - 4) / Math.max(1, rawLines.length));
        return {
            status: 'synced',
            lines: rawLines.map((text, idx) => ({ time: 2 + idx * step, text }))
        };
    }
    return { status: 'empty', lines: [] };
}

export function createLyrics({ player, toggle, stage, panel, status, lines: container, follow, preview, previewLine, headerTitle, headerSubtitle }) {
    let open = false, key = '', result = { status: 'idle', lines: [] }, active = -1, following = true;
    let controller = null, disposed = false;
    const cache = new Map();
    const prevEl = document.getElementById('lyrics-preview-prev');
    const nextEl = document.getElementById('lyrics-preview-next');
    const barEl = document.getElementById('lyrics-ticker-bar');

    const isLyricsTab = () => state.expandedPlayer && player.dataset.expandedTab === 'lyrics' && !disposed;
    const expanded = () => state.expandedPlayer && !disposed;
    const trackKey = () => state.room?.track ? `${state.room.track.id || ''}|${state.room.track.title || ''}|${state.room.track.url || ''}` : '';

    function render() {
        const showFull = isLyricsTab();
        open = showFull;
        player.classList.toggle('has-lyrics', showFull);
        panel.hidden = !showFull;
        stage.hidden = !showFull;
        panel.dataset.state = result.status;
        if (toggle) toggle.hidden = !showFull;

        const hasTrack = Boolean(state.room?.track);
        const showBoxUnderArt = expanded() && !showFull && hasTrack;
        if (preview) {
            preview.hidden = !showBoxUnderArt;
            preview.disabled = !hasTrack;
            preview.setAttribute('aria-expanded', String(showFull));
        }
        player.classList.toggle('has-lyric-preview', showBoxUnderArt);

        if (headerTitle?.parentElement) {
            headerTitle.parentElement.hidden = !showFull;
            headerTitle.textContent = showFull ? state.room?.track?.title || 'Track lyrics' : '';
            if (headerSubtitle) headerSubtitle.textContent = showFull ? state.room?.track?.artist || '' : '';
        }

        const isAr = state.language !== 'en';
        const messages = {
            idle: isAr ? 'الكلمات غير متوفرة لهذه الأغنية' : 'Lyrics Not Available For This Song',
            loading: isAr ? 'جاري تحميل ومزامنة كلمات الأغنية...' : 'Loading synced lyrics…',
            empty: isAr ? 'الكلمات غير متوفرة لهذه الأغنية' : 'Lyrics Not Available For This Song',
            unsupported: isAr ? 'الكلمات غير متوفرة لهذه الأغنية' : 'Lyrics Not Available For This Song',
            plain: isAr ? 'كلمات الأغنية' : 'Lyrics',
            synced: isAr ? 'كلمات متزامنة' : 'Synced lyrics'
        };
        status.textContent = messages[result.status] || messages.idle;
        status.hidden = result.status === 'synced' && result.lines.length > 0;

        container.replaceChildren(...result.lines.map((line, index) => {
            const node = document.createElement('p');
            node.className = 'lyric-line';
            node.textContent = line.text || '♪';
            node.dataset.index = String(index);
            if (Number.isFinite(line.time)) {
                node.dataset.time = String(line.time);
                node.addEventListener('click', () => {
                    if (canControl()) send('seek', { positionSeconds: Math.max(0, line.time) });
                });
            }
            return node;
        }));
        container.style.paddingBlock = result.status === 'synced' ? `${Math.max(24, panel.clientHeight / 2 - 32)}px` : '24px';
        active = -1;
        following = true;
        if (follow) follow.hidden = true;
        panel.scrollTop = 0;
        update(playbackPosition(), true);
    }

    async function load() {
        if (result.status !== 'idle' || !key || !state.room?.track) return;
        if (cache.has(key)) {
            result = cache.get(key);
            render();
            return;
        }
        result = { status: 'loading', lines: [] };
        render();
        controller = new AbortController();
        const request = controller, requestedKey = key;
        try {
            const data = await lyricsAPI(state.room.track, request.signal);
            if (request.signal.aborted || requestedKey !== key || disposed) return;
            result = normalizeLyrics(data);
            cache.set(key, result);
            if (cache.size > 40) cache.delete(cache.keys().next().value);
        } catch {
            if (request.signal.aborted || requestedKey !== key || disposed) return;
            result = { status: 'empty', lines: [] };
        }
        render();
    }

    function sync() {
        const next = trackKey();
        if (next !== key) {
            controller?.abort();
            key = next;
            result = { status: 'idle', lines: [] };
            render();
        }
        if (key && result.status === 'idle') {
            load();
        }
    }

    function update(position, force = false) {
        const isAr = state.language !== 'en';
        if (result.status === 'loading') {
            if (prevEl) prevEl.textContent = '';
            if (previewLine) previewLine.textContent = isAr ? 'جاري تحميل كلمات الأغنية...' : 'Loading synced lyrics…';
            if (nextEl) nextEl.textContent = '';
            if (barEl) barEl.style.width = '0%';
            return;
        }
        if (result.status !== 'synced' || !result.lines.length) {
            if (prevEl) prevEl.textContent = '';
            if (previewLine) {
                previewLine.textContent = result.status === 'plain' && result.lines.length
                    ? result.lines[0].text
                    : (isAr ? '♪ موسيقى (لا توجد كلمات مسجلة)' : '♪ Instrumental / No lyrics available');
            }
            if (nextEl) nextEl.textContent = '';
            if (barEl) barEl.style.width = '0%';
            return;
        }

        let next = -1;
        for (let i = 0; i < result.lines.length && result.lines[i].time <= position + 0.08; i++) {
            next = i;
        }

        const currentObj = next >= 0 ? result.lines[next] : null;
        const prevObj = next > 0 ? result.lines[next - 1] : null;
        const nextObj = next + 1 < result.lines.length ? result.lines[next + 1] : null;

        if (prevEl) prevEl.textContent = prevObj?.text || '';
        if (previewLine) previewLine.textContent = currentObj?.text || '♪ • • •';
        if (nextEl) nextEl.textContent = nextObj?.text || '';

        if (barEl) {
            const startTime = currentObj ? currentObj.time : 0;
            const endTime = nextObj ? nextObj.time : (Number(state.room?.track?.duration) || startTime + 4);
            const span = Math.max(0.25, endTime - startTime);
            const pct = Math.max(0, Math.min(100, ((position - startTime) / span) * 100));
            barEl.style.width = `${pct.toFixed(1)}%`;
        }

        if (next === active && !force) return;
        container.children[active]?.classList.remove('is-current');
        container.children[active]?.removeAttribute('aria-current');
        active = next;
        const node = container.children[active];
        if (node) {
            node.classList.add('is-current');
            node.setAttribute('aria-current', 'true');
            if (isLyricsTab() && following) {
                const top = panel.scrollTop + node.getBoundingClientRect().top - panel.getBoundingClientRect().top - panel.clientHeight / 2 + node.clientHeight / 2;
                panel.scrollTo({ top: Math.max(0, top), behavior: force || matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
            }
        }
    }

    function stopFollowing() {
        if (result.status === 'synced') {
            following = false;
            if (follow) follow.hidden = !isLyricsTab();
        }
    }

    panel.addEventListener('wheel', stopFollowing, { passive: true });
    panel.addEventListener('touchstart', stopFollowing, { passive: true });
    panel.addEventListener('pointerdown', stopFollowing);
    panel.addEventListener('keydown', event => {
        if (['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' '].includes(event.key)) stopFollowing();
    });

    if (follow) {
        follow.addEventListener('click', () => {
            panel.focus({ preventScroll: true });
            following = true;
            follow.hidden = true;
            update(playbackPosition(), true);
        });
    }

    const resize = new ResizeObserver(() => {
        container.style.paddingBlock = result.status === 'synced' ? `${Math.max(24, panel.clientHeight / 2 - 32)}px` : '24px';
        if (following) update(playbackPosition(), true);
    });
    resize.observe(panel);
    on('room', sync);
    on('progress', update);
    on('language', () => render());
    on('dispose', () => {
        disposed = true;
        controller?.abort();
        resize.disconnect();
        cache.clear();
    });
    return {
        viewChanged() {
            render();
            sync();
        }
    };
}
