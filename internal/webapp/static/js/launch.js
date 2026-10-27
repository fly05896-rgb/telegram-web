const storageKey = 'telemusic:room-session';
const guestIdKey = 'telemusic:guest-id';

function getOrCreateGuestToken() {
    try {
        let existing = localStorage.getItem(guestIdKey);
        if (existing && /^guest:[a-zA-Z0-9_-]+$/.test(existing)) return existing;
        const rand = Math.random().toString(36).slice(2, 10) + Date.now().toString(36);
        existing = 'guest:' + rand;
        localStorage.setItem(guestIdKey, existing);
        return existing;
    } catch {
        return 'guest:web_' + Math.floor(100000 + Math.random() * 900000);
    }
}

function hashGuestRoomId(guestToken) {
    let h = 2166136261;
    for (let i = 0; i < guestToken.length; i++) {
        h ^= guestToken.charCodeAt(i);
        h = Math.imul(h, 16777619);
    }
    return String(9000000000 + (Math.abs(h) % 899999999));
}

export function readLaunch(telegram) {
    const params = new URLSearchParams(location.search);
    const fragment = new URLSearchParams(location.hash.slice(1));
    let saved = null;
    try { saved = JSON.parse(sessionStorage.getItem(storageKey)); } catch { /* Storage is optional. */ }
    let transferred = null;
    try { transferred = JSON.parse(fragment.get('browser_session')); } catch { /* Ignore malformed handoffs. */ }

    let nativeData = telegram?.initData?.trim() ? telegram.initData : '';
    if (!nativeData && fragment.get('tgWebAppData')) {
        nativeData = fragment.get('tgWebAppData');
    }

    const source = transferred || (nativeData ? { ...(saved?.initData === nativeData ? saved : {}), initData: nativeData } : saved);
    let initData = typeof source?.initData === 'string' && source.initData ? source.initData : '';

    const signed = new URLSearchParams(initData);
    let user = {};
    try { user = JSON.parse(signed.get('user')) || {}; } catch { /* Display data never grants access. */ }
    const isTelegramWebApp = Boolean(!transferred && nativeData && telegram?.platform && telegram.platform !== 'unknown');
    if (isTelegramWebApp && telegram?.initDataUnsafe?.user) {
        user = { ...telegram.initDataUnsafe.user };
    }

    let startParam = String(
        signed.get('start_param') ||
        (nativeData && !transferred ? telegram?.initDataUnsafe?.start_param : '') ||
        params.get('tgWebAppStartParam') ||
        params.get('startapp') ||
        params.get('chat_id') ||
        params.get('room') ||
        source?.roomId ||
        user.id ||
        ''
    ).trim();

    if (!initData) {
        const guestToken = getOrCreateGuestToken();
        initData = guestToken;
        const guestRoom = hashGuestRoomId(guestToken);
        if (!startParam || startParam === '0' || !/^-?\d+$/.test(startParam)) {
            startParam = guestRoom;
        }
        user = {
            id: Number(guestRoom),
            first_name: 'Web Listener',
            username: 'telemusic_web'
        };
    }

    const partnerInvite = fragment.get('partner_invite') || source?.partnerInvite || '';
    const roomId = partnerInvite
        ? String(fragment.get('room') || source?.roomId || startParam || '9000000001')
        : /^pm_([1-9]\d*)_[A-Za-z0-9_-]+$/.exec(startParam)?.[1] || startParam || '9000000001';

    const launch = {
        initData,
        roomId: /^-?\d+$/.test(String(roomId)) && String(roomId) !== '0' ? String(roomId) : '9000000001',
        partnerInvite,
        user,
        isTelegramWebApp,
        isBrowserHandoff: Boolean(transferred),
        isPartnerJoin: fragment.has('partner_invite')
    };
    rememberLaunch(launch);
    if (location.search || location.hash) history.replaceState(history.state, '', location.pathname);
    return launch;
}

export function rememberLaunch({ initData, roomId, partnerInvite }) {
    try {
        if (initData) sessionStorage.setItem(storageKey, JSON.stringify({ initData, roomId, partnerInvite }));
    } catch { /* The current session still works without storage. */ }
}

export function browserSessionURL(launch) {
    const url = new URL('/room', location.origin);
    url.hash = new URLSearchParams({ browser_session: JSON.stringify({ initData: launch.initData, roomId: launch.roomId, partnerInvite: launch.partnerInvite }) }).toString();
    return url.href;
}
