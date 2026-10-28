const storageKey = 'telemusic:room-session';
const tgUidKey = 'telemusic:tg-uid';
const tgUserKey = 'telemusic:tg-user';

export function getSavedTelegramUid() {
    try {
        const raw = localStorage.getItem(tgUidKey);
        if (raw && /^[1-9]\d{4,15}$/.test(raw.trim())) {
            return raw.trim();
        }
    } catch { /* Storage is optional */ }
    return '';
}

export function getSavedTelegramUser() {
    try {
        const raw = localStorage.getItem(tgUserKey);
        if (raw) return JSON.parse(raw);
    } catch { /* Ignore */ }
    return null;
}

export function saveTelegramIdSession(uid, roomId, userObj = null) {
    const cleanUid = String(uid || '').trim();
    if (!/^[1-9]\d{4,15}$/.test(cleanUid)) return null;
    const targetRoom = roomId && /^-?\d+$/.test(String(roomId)) && String(roomId) !== '0'
        ? String(roomId)
        : cleanUid;
    const user = userObj || {
        id: Number(cleanUid),
        first_name: 'Telegram User',
        username: ''
    };
    try {
        localStorage.setItem(tgUidKey, cleanUid);
        localStorage.setItem(tgUserKey, JSON.stringify(user));
    } catch { /* Ignore */ }
    const sessionData = {
        initData: `uid:${cleanUid}`,
        roomId: targetRoom,
        partnerInvite: ''
    };
    rememberLaunch(sessionData);
    return { ...sessionData, user };
}

export function clearTelegramIdSession() {
    try {
        localStorage.removeItem(tgUidKey);
        localStorage.removeItem(tgUserKey);
        sessionStorage.removeItem(storageKey);
    } catch { /* Ignore */ }
}

function cleanRoomParam(raw) {
    if (!raw) return '';
    let s = String(raw).trim();
    if (s.startsWith('room_')) s = s.slice(5);
    else if (s.startsWith('player_')) s = s.slice(7);
    return s;
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

    const urlUid = (params.get('uid') || fragment.get('uid') || '').trim();
    if (!nativeData && /^[1-9]\d{4,15}$/.test(urlUid)) {
        try { localStorage.setItem(tgUidKey, urlUid); } catch { /* Ignore */ }
    }

    const source = transferred || (nativeData ? { ...(saved?.initData === nativeData ? saved : {}), initData: nativeData } : saved);
    let initData = typeof source?.initData === 'string' && source.initData ? source.initData : '';

    // Do not reuse old anonymous guest tokens; Chrome requires verified Telegram ID
    if (initData.startsWith('guest:')) {
        initData = '';
    }

    const signed = new URLSearchParams(initData);
    let user = {};
    try { user = JSON.parse(signed.get('user')) || {}; } catch { /* Display data never grants access. */ }
    const isTelegramWebApp = Boolean(!transferred && nativeData && telegram?.platform && telegram.platform !== 'unknown');
    if (isTelegramWebApp && telegram?.initDataUnsafe?.user) {
        user = { ...telegram.initDataUnsafe.user };
    }

    const savedUid = getSavedTelegramUid();
    if (!initData && savedUid) {
        initData = `uid:${savedUid}`;
        const savedUser = getSavedTelegramUser();
        user = savedUser ? {
            id: Number(savedUser.id || savedUid),
            first_name: savedUser.firstName || savedUser.first_name || 'Telegram User',
            last_name: savedUser.lastName || savedUser.last_name || '',
            username: savedUser.username || ''
        } : {
            id: Number(savedUid),
            first_name: 'Telegram User',
            username: ''
        };
    } else if (initData.startsWith('uid:')) {
        const uidNum = Number(initData.slice(4));
        const savedUser = getSavedTelegramUser();
        user = savedUser ? {
            id: Number(savedUser.id || uidNum),
            first_name: savedUser.firstName || savedUser.first_name || 'Telegram User',
            last_name: savedUser.lastName || savedUser.last_name || '',
            username: savedUser.username || ''
        } : {
            id: uidNum,
            first_name: 'Telegram User',
            username: ''
        };
    }

    let startParam = cleanRoomParam(
        params.get('room') ||
        params.get('chat_id') ||
        params.get('startapp') ||
        params.get('tgWebAppStartParam') ||
        signed.get('start_param') ||
        (nativeData && !transferred ? telegram?.initDataUnsafe?.start_param : '') ||
        source?.roomId ||
        user.id ||
        savedUid ||
        ''
    );

    const partnerInvite = fragment.get('partner_invite') || source?.partnerInvite || '';
    const rawRoomId = partnerInvite
        ? String(cleanRoomParam(fragment.get('room') || source?.roomId || startParam))
        : /^pm_([1-9]\d*)_[A-Za-z0-9_-]+$/.exec(startParam)?.[1] || startParam || (user.id ? String(user.id) : '');

    const launch = {
        initData,
        roomId: /^-?\d+$/.test(String(rawRoomId)) && String(rawRoomId) !== '0' ? String(rawRoomId) : (savedUid || '0'),
        partnerInvite,
        user,
        isTelegramWebApp,
        isBrowserHandoff: Boolean(transferred),
        isPartnerJoin: fragment.has('partner_invite')
    };
    if (launch.initData) {
        rememberLaunch(launch);
    }
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
    if (launch.roomId && launch.roomId !== '0') {
        url.searchParams.set('room', launch.roomId);
    }
    if (launch.user?.id) {
        url.searchParams.set('uid', String(launch.user.id));
    }
    url.hash = new URLSearchParams({ browser_session: JSON.stringify({ initData: launch.initData, roomId: launch.roomId, partnerInvite: launch.partnerInvite }) }).toString();
    return url.href;
}
