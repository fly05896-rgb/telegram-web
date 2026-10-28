import { state, validPasscode, platform, telegram, on, haptic, connect, joinWithPassword, applyChromeTelegramSession, safeURL, formatTime } from './core.js?v=88';
import { setTrackArtwork } from './artwork.js?v=88';

export function createSessionScreen({ startListening }) {
    const screen = document.getElementById('session-notice');
    const title = document.getElementById('session-title');
    const description = document.getElementById('session-description');
    const status = document.getElementById('session-status');
    const steps = document.getElementById('session-steps');
    const primary = document.getElementById('session-primary');
    const primaryLabel = document.getElementById('session-primary-label');
    const primaryIcon = document.getElementById('session-primary-icon');
    const audioNote = document.getElementById('session-audio-note');
    const emptyPreview = document.getElementById('session-empty');
    const secondary = document.getElementById('session-secondary');
    const hint = document.getElementById('session-hint');
    const app = document.getElementById('app');
    const passwordForm = document.getElementById('partner-join-form');
    const passwordInput = document.getElementById('partner-join-password');
    const passwordError = document.getElementById('partner-join-error');
    const passwordButton = document.getElementById('partner-join-button');
    const passwordLabel = document.getElementById('partner-join-label');
    const tgIdForm = document.getElementById('telegram-id-form');
    const tgIdInput = document.getElementById('telegram-id-input');
    const tgIdRoomInput = document.getElementById('telegram-id-room-input');
    const tgIdError = document.getElementById('telegram-id-error');
    const tgIdSubmit = document.getElementById('telegram-id-submit');
    const tgIdSubmitLabel = document.getElementById('telegram-id-submit-label');
    const profile = document.getElementById('session-profile');
    const profileName = document.getElementById('session-profile-name');
    const profileHandle = document.getElementById('session-profile-handle');
    const profilePhoto = document.getElementById('session-profile-photo');
    const profileInitial = document.getElementById('session-profile-initial');
    const trackPreview = document.getElementById('session-track');
    const trackTitle = document.getElementById('session-track-title');
    const trackMeta = document.getElementById('session-track-meta');
    const trackStatus = document.getElementById('session-track-status');
    const cover = document.getElementById('session-cover');
    const record = screen.querySelector('.session-record');
    const writeNotice = document.getElementById('write-access-notice');
    let writePromptDismissed = false;
    let retryable = false;
    let admitted = false;
    let hadWriteAccess = false;
    let currentCode = 'connecting';
    function dismissWritePrompt() {
        writePromptDismissed = true;
        writeNotice.close();
        primary.focus({ preventScroll: true });
    }
    document.getElementById('write-access-dismiss').addEventListener('click', dismissWritePrompt);
    writeNotice.addEventListener('cancel', event => { event.preventDefault(); dismissWritePrompt(); });
    function syncWritePrompt() {
        const needed = state.permissions.userId && !state.permissions.allowsWriteToPM;
        const visible = currentCode === 'listening_required' && admitted && !screen.hidden;
        if (needed && visible && !writePromptDismissed) {
            if (!writeNotice.open) writeNotice.showModal();
        } else if (writeNotice.open) {
            writeNotice.close();
            if (visible) primary.focus({ preventScroll: true });
        }
    }
    const fromGroup = ['Return to the group this room belongs to.', 'Open its player button there, instead of a copied link.'];
    const states = {
        connecting: { title: 'جاري تجهيز\nالمشغل الخاص بك', description: 'يتم الاتصال بالغرفة وتحميل قائمة التشغيل...', status: 'جاري الاتصال', loading: true },
        listening_required: { title: 'الموسيقى جاهزة\nللتشغيل', description: 'اضغط بالأسفل لبدء الاستماع ومزامنة الصوت.', status: 'جاهز للاستماع', action: 'تشغيل المشغل' },
        partner_password_required: { title: 'استمعوا معاً', description: 'أدخل كلمة المرور الخاصة بالغرفة للمشاركة.', status: 'غرفة خاصة' },
        group_launch_required: { title: 'This room opens\nfrom its group.', description: 'This player belongs to a Telegram group. Open it from that group to play music.', status: 'Group access required', steps: fromGroup },
        group_context_mismatch: { title: 'Right link.\nDifferent chat.', description: 'You opened a link for another group. Return to the group it belongs to and open the player there.', status: 'Different chat', steps: fromGroup },
        group_context_missing: { title: 'Open this room\ninside its group.', description: 'This launch didn’t include the group it came from. Open a fresh player button inside the linked Telegram group.', status: 'Group launch needed', steps: fromGroup },
        chat_binding_required: { title: 'One quick setup.\nThen press play.', description: 'This group needs to set up its player once. Everyone in the group can use it afterward.', status: 'Group setup needed', steps: ['Return to the linked Telegram group.', 'On a fresh bot message, tap Verify group for player, then Open Web App Player.'] },
        chat_binding_unavailable: { title: 'Your room is\nout of reach.', description: 'We couldn’t confirm access to this room right now. Give it a moment, then try again.', status: 'Temporarily unavailable', retry: 'Try again' },
        telegram_authentication_required: { title: 'تسجيل الدخول\nعبر أيدي تيليجرام', description: 'أرسل /start للبوت في تيليجرام أولاً ثم أدخل الأيدي (ID) الخاص بك بالأسفل لتشغيل المشغل من متصفح كروم بدون انقطاع.', status: 'دخول متصفح كروم', chromeLogin: true },
        telegram_id_not_started: { title: 'أرسل /start\nللبوت أولاً', description: 'يجب إرسال /start للبوت @Fmusic_TTbot في تيليجرام أولاً ليتعرف عليك ويرسل لك إشعارات التشغيل، ثم أدخل الأيدي هنا.', status: 'مطلوب تفعيل /start', chromeLogin: true },
        telegram_id_invalid: { title: 'أيدي تيليجرام\nغير صحيح', description: 'تأكد من كتابة الأيدي الرقمي الصحيح الذي يظهر لك بعد إرسال /start للبوت.', status: 'تحقق من الأيدي', chromeLogin: true },
        private_room_owner_required: { title: 'This is someone\nelse’s room.', description: 'Enter the room passcode or open your own room.', status: 'Personal room' },
        partner_invite_expired: { title: 'Time for a\nfresh invite.', description: 'This partner invite expired or is no longer available. Ask the room owner for a new link.', status: 'Invite expired' },
        partner_invite_full: { title: 'This invite\nis full.', description: 'This partner invite has reached its user limit. Ask the room owner for a new link.', status: 'User limit reached' },
        invalid_room_identity: { title: 'تسجيل الدخول\nإلى المشغل', description: 'أدخل الأيدي الخاص بك في تيليجرام لفتح المشغل الخاص بك أو الانضمام لغرفة.', status: 'دخول متصفح كروم', chromeLogin: true },
        signed_room_mismatch: { title: 'Open a fresh\nroom link.', description: 'This link doesn’t match the room in your Telegram session. Open a new player button from the chat you want to join.', status: 'Room link changed', steps: fromGroup },
        telegram_required: { title: 'مشغل TeleMusic\nعلى متصفح كروم', description: 'لتشغيل الموسيقى في الخلفية من متصفح كروم، أرسل /start للبوت في تيليجرام ثم أدخل الأيدي (ID) الخاص بك هنا.', status: 'دخول متصفح كروم', chromeLogin: true },
        session_in_use: { title: 'المشغل مفتوح\nفي نافذة أخرى', description: 'اضغط على الزر بالأسفل لنقل الاستماع إلى هذه النافذة.', status: 'جلسة أخرى نشطة', action: 'استخدام هذه النافذة' },
        session_moved: { title: 'تم نقل الجلسة\nإلى جهاز آخر', description: 'اضغط بالأسفل لاستعادة التشغيل هنا.', status: 'يعمل في نافذة أخرى', action: 'تشغيل هنا' },
        connection_unavailable: { title: 'جاري إعادة\nالاتصال...', description: 'نحاول استعادة الاتصال بالغرفة الآن.', status: 'إعادة الاتصال', retry: 'إعادة المحاولة' }
    };

    function setImage(container, value) {
        const url = safeURL(value);
        if (!url) { container.hidden = true; container.replaceChildren(); container.dataset.source = ''; container.dataset.failed = ''; return; }
        if (container.dataset.source !== url) {
            container.dataset.source = url; container.dataset.failed = '';
            const image = document.createElement('img'); image.alt = '';
            if (container === cover) setTrackArtwork(image, url); else image.src = url;
            container.replaceChildren(image);
        }
        container.hidden = container.dataset.failed === 'true';
    }
    profilePhoto.addEventListener('error', () => { profilePhoto.dataset.failed = 'true'; profilePhoto.hidden = true; profileInitial.hidden = false; }, true);
    function renderListeningContext() {
        if (currentCode !== 'listening_required' || !admitted || screen.hidden) return;
        const firstName = typeof state.user.first_name === 'string' ? state.user.first_name.trim() : '';
        const lastName = typeof state.user.last_name === 'string' ? state.user.last_name.trim() : '';
        const username = typeof state.user.username === 'string' ? state.user.username.trim() : '';
        const name = [firstName, lastName].filter(Boolean).join(' ') || 'Listener';
        profile.hidden = false;
        profileName.textContent = name; profileName.title = name;
        profileHandle.textContent = username ? `@${username}` : ''; profileHandle.hidden = !username;
        profileInitial.textContent = Array.from(firstName || name)[0].toUpperCase();
        setImage(profilePhoto, state.user.photo_url);
        profileInitial.hidden = !profilePhoto.hidden;
        const track = state.room?.track;
        const preparing = track?.ready === false;
        const playing = Boolean(track && !preparing && state.room.playback?.status === 'playing');
        const playback = !track ? 'idle' : preparing ? 'preparing' : playing ? 'playing' : 'paused';
        screen.dataset.playback = playback;
        trackPreview.hidden = !track;
        emptyPreview.hidden = Boolean(track);
        title.textContent = !track ? 'ابحث عن أغنيتك\nالمفضلة'
            : preparing ? 'جاري تحضير\nالأغنية بسرعة البرق'
            : playing ? 'اضغط للاستماع\nالآن' : 'الموسيقى متوقفة\nمؤقتاً';
        description.textContent = !track ? 'ابحث وشغل وشارك الموسيقى.'
            : preparing ? 'ثوانٍ قليلة وتبدأ الأغنية.'
            : playing ? 'استمع مع الجميع في نفس اللحظة.'
            : 'تصفح قائمة التشغيل أو استأنف الأغنية.';
        const needsWriteAccess = !state.permissions.allowsWriteToPM;
        primaryLabel.textContent = needsWriteAccess ? playing ? 'متابعة الاستماع' : 'تصفح الموسيقى' : !track ? 'استكشف الموسيقى' : playing ? 'استمع الآن' : 'فتح المشغل';
        primaryIcon.setAttribute('href', `/static/assets/icons.svg?v=88#${!track ? 'search' : playing ? 'volume' : 'music'}`);
        audioNote.textContent = playing ? 'الصوت متزامن ويعمل في الخلفية.' : !track ? 'ابحث عن أي أغنية أو رابط يوتيوب.' : 'سيبدأ الصوت فور تشغيل الأغنية.';
        audioNote.hidden = false;
        if (track) {
            trackTitle.textContent = track.title || 'Untitled track'; trackTitle.title = trackTitle.textContent;
            trackMeta.textContent = [track.artist || (track.user ? `Added by ${track.user}` : track.platform), Number(track.duration) > 0 ? formatTime(track.duration) : ''].filter(Boolean).join(' · ');
            trackMeta.hidden = !trackMeta.textContent;
            trackStatus.textContent = preparing ? 'Getting ready…' : playing ? 'Now playing' : 'Paused';
            trackPreview.dataset.playing = String(playing);
        }
        setImage(cover, track?.thumbnail);
        record.toggleAttribute('hidden', !cover.hidden);
        syncWritePrompt();
    }
    on('room', renderListeningContext);
    on('permissions', () => {
        const revoked = state.permissions.userId && hadWriteAccess && !state.permissions.allowsWriteToPM;
        if (revoked) writePromptDismissed = false;
        if (state.permissions.userId) hadWriteAccess = state.permissions.allowsWriteToPM;
        if (admitted && revoked && screen.hidden) show('listening_required');
        else renderListeningContext();
    });

    function show(code, message) {
        currentCode = code;
        const view = states[code] || { ...states.telegram_authentication_required, description: typeof message === 'string' && message ? message : states.telegram_authentication_required.description };
        retryable = Boolean(view.retry);
        title.textContent = view.title;
        description.textContent = typeof message === 'string' && message && ['telegram_id_not_started', 'telegram_id_invalid'].includes(code) ? message : view.description;
        status.textContent = view.status;
        const passwordRequired = code === 'partner_password_required';
        const chromeLogin = Boolean(view.chromeLogin) && !platform.isTelegramWebApp;
        screen.classList.toggle('passcode-entry', passwordRequired || chromeLogin);
        passwordForm.hidden = !passwordRequired;
        if (tgIdForm) {
            tgIdForm.hidden = !chromeLogin;
            if (chromeLogin && tgIdRoomInput && state.roomId && state.roomId !== '0' && !tgIdRoomInput.value) {
                tgIdRoomInput.value = state.roomId;
            }
            if (chromeLogin && typeof message === 'string' && message && ['telegram_id_not_started', 'telegram_id_invalid'].includes(code)) {
                tgIdError.textContent = message;
                tgIdError.hidden = false;
            }
        }
        screen.dataset.state = code === 'listening_required' ? 'listening' : view.loading ? 'connecting' : 'blocked';
        screen.setAttribute('aria-busy', String(Boolean(view.loading)));
        profile.hidden = true;
        emptyPreview.hidden = true;
        audioNote.hidden = true;
        delete screen.dataset.playback;
        trackPreview.hidden = true;
        cover.hidden = true;
        record.removeAttribute('hidden');
        steps.replaceChildren(...(view.steps || []).map(copy => { const item = document.createElement('li'); item.textContent = copy; return item; }));
        steps.hidden = !view.steps || chromeLogin;
        primary.hidden = Boolean(view.loading) || passwordRequired || chromeLogin;
        primary.disabled = false;
        primaryLabel.textContent = view.action || view.retry || (platform.isTelegramWebApp ? 'العودة إلى تيليجرام' : 'العودة للرئيسية');
        primaryIcon.setAttribute('href', `/static/assets/icons.svg?v=88#${view.retry ? 'restart' : view.action ? 'volume' : 'logout'}`);
        secondary.hidden = (!view.retry && !view.action && !passwordRequired) || chromeLogin;
        secondary.textContent = platform.isTelegramWebApp ? 'العودة إلى تيليجرام' : 'العودة للرئيسية';
        hint.hidden = true;
        screen.hidden = false;
        app.hidden = true;
        document.body.classList.add('session-blocked');
        for (const node of document.body.children) if (node !== screen && node.tagName !== 'SCRIPT') node.inert = true;
        for (const dialog of document.querySelectorAll('dialog[open]')) dialog.close();
        renderListeningContext();
        if (!view.loading) {
            if (chromeLogin && tgIdInput) tgIdInput.focus({ preventScroll: true });
            else (code === 'listening_required' ? writeNotice.open ? writeNotice.querySelector('[data-action="write-access"]') : primary : title).focus({ preventScroll: true });
            if (code !== 'listening_required' && !passwordRequired && !chromeLogin) haptic('warning');
        }
    }

    if (tgIdForm && tgIdInput && tgIdSubmit) {
        tgIdInput.addEventListener('input', () => {
            tgIdError.hidden = true;
            tgIdInput.setAttribute('aria-invalid', 'false');
        });
        tgIdForm.addEventListener('submit', async event => {
            event.preventDefault();
            const rawId = tgIdInput.value.trim();
            const rawRoom = tgIdRoomInput ? tgIdRoomInput.value.trim() : '';
            if (!/^[1-9]\d{4,15}$/.test(rawId)) {
                tgIdError.textContent = 'أدخل أيدي تيليجرام صحيح مكون من أرقام فقط (مثال: 8313557781).';
                tgIdError.hidden = false;
                tgIdInput.setAttribute('aria-invalid', 'true');
                return;
            }
            const targetRoom = /^-?\d+$/.test(rawRoom) && rawRoom !== '0'
                ? rawRoom
                : (state.roomId && state.roomId !== '0' ? state.roomId : rawId);

            tgIdSubmit.disabled = true;
            if (tgIdSubmitLabel) tgIdSubmitLabel.textContent = 'جاري التحقق من الأيدي...';
            tgIdError.hidden = true;

            try {
                const resp = await fetch(`/api/verify-id?user_id=${encodeURIComponent(rawId)}&room_id=${encodeURIComponent(targetRoom)}`);
                const result = await resp.json();
                if (!resp.ok || !result.ok) {
                    tgIdError.textContent = result.error || 'أرسل /start للبوت في تيليجرام أولاً ثم أدخل الأيدي الخاص بك هنا.';
                    tgIdError.hidden = false;
                    tgIdInput.setAttribute('aria-invalid', 'true');
                    tgIdSubmit.disabled = false;
                    if (tgIdSubmitLabel) tgIdSubmitLabel.textContent = 'دخول المشغل (Connect Chrome Player)';
                    return;
                }
                tgIdSubmit.disabled = false;
                if (tgIdSubmitLabel) tgIdSubmitLabel.textContent = 'دخول المشغل (Connect Chrome Player)';
                admitted = false;
                show('connecting');
                applyChromeTelegramSession(result.user.id, targetRoom, result.user);
            } catch {
                tgIdError.textContent = 'تعذر الاتصال بالسيرفر، حاول مرة أخرى.';
                tgIdError.hidden = false;
                tgIdSubmit.disabled = false;
                if (tgIdSubmitLabel) tgIdSubmitLabel.textContent = 'دخول المشغل (Connect Chrome Player)';
            }
        });
    }

    function joining(value) {
        passwordForm.setAttribute('aria-busy', String(value));
        passwordInput.readOnly = value;
        passwordButton.disabled = value || !validPasscode(passwordInput.value);
        passwordLabel.textContent = value ? 'Joining…' : 'Join player';
    }
    passwordInput.addEventListener('input', () => {
        passwordInput.setAttribute('aria-invalid', 'false'); passwordError.hidden = true;
        joining(false);
    });
    passwordForm.addEventListener('submit', event => {
        event.preventDefault();
        if (passwordButton.disabled) return;
        if (!validPasscode(passwordInput.value)) { passwordError.textContent = 'Use 4–64 characters without leading or trailing spaces.'; passwordError.hidden = false; return; }
        passwordError.hidden = true;
        if (joinWithPassword(passwordInput.value)) joining(true);
    });
    on('partner-password-required', error => {
        show('partner_password_required');
        joining(false);
        passwordInput.setAttribute('aria-invalid', String(error.invalid));
        passwordError.textContent = error.invalid ? error.message : '';
        passwordError.hidden = !error.invalid;
        if (error.invalid) haptic('error');
        passwordInput.focus();
    });
    on('request-error', error => {
        if (error.type !== 'join' || currentCode !== 'partner_password_required') return;
        joining(false);
        passwordError.textContent = String(error.message); passwordError.hidden = false;
    });

    function back() {
        if (!platform.isTelegramWebApp) { location.assign('/'); return; }
        try {
            if (typeof telegram.close === 'function') { telegram.close(); return; }
        } catch { /* Let the user close the native window if this client cannot. */ }
        hint.textContent = 'Close this Mini App with Telegram’s close button, then return to your chat.';
        hint.hidden = false;
    }

    screen.addEventListener('click', event => {
        const button = event.target.closest('button[data-session-action]');
        if (!button || button.disabled) return;
        haptic('selection');
        if (button === primary && currentCode === 'listening_required') { startListening(); }
        else if (button === primary && ['session_in_use', 'session_moved'].includes(currentCode)) {
            state.sessionFailure = null;
            state.stopped = false;
            admitted = false;
            show('connecting');
            connect({ takeOver: true });
        }
        else if (button === primary && retryable) { primary.disabled = true; location.reload(); }
        else back();
    });
    on('back', () => { if (writeNotice.open) dismissWritePrompt(); else if (!screen.hidden) back(); });
    function reveal() {
        if (screen.hidden) return;
        screen.hidden = true;
        if (writeNotice.open) writeNotice.close();
        app.hidden = false;
        document.body.classList.remove('session-blocked');
        for (const node of document.body.children) if (node !== screen && node.tagName !== 'SCRIPT') node.inert = false;
        if (screen.contains(document.activeElement)) document.getElementById('workspace').focus({ preventScroll: true });
    }
    on('session-ready', () => {
        passwordForm.reset();
        admitted = true;
        state.joinedListening = true;
        reveal();
    });
    on('listening', () => {
        if (!admitted || state.stopped || state.sessionFailure) return;
        if (state.joinedListening && currentCode === 'listening_required') reveal();
        else if (!state.joinedListening && state.audioBlocked) show('listening_required');
    });
    on('authentication-failed', error => show(error.code === 'invalid_start_param' ? 'signed_room_mismatch' : error.code, error.message));
    on('session-ended', message => show('session_moved', message));
    on('connection', () => {
        if (admitted || state.stopped) return;
        if (state.connection === 'reconnecting') show('connection_unavailable');
        else if (['connecting', 'connected'].includes(state.connection)) show('connecting');
    });
    return { show };
}
