/* ============================================================
   MarketPulse_RU — Settings Page (Admin)
   ============================================================ */
(function () {
    'use strict';

    var ref       = Vue.ref;
    var onMounted = Vue.onMounted;
    var onBeforeUnmount = Vue.onBeforeUnmount;

    window.PageSettings = {
        name: 'PageSettings',
        setup: function () {
            var tgState      = ref('idle');
            var tgMessage    = ref('');
            var tgError      = ref('');
            var authCode     = ref('');
            var authPassword = ref('');
            var submitting   = ref(false);
            var pollTimer    = null;

            async function fetchTelegramStatus() {
                try {
                    var data = await API.get('/api/admin/telegram-status');
                    tgState.value = data.state || 'idle';
                    tgMessage.value = data.message || '';
                    tgError.value = data.error || '';
                } catch (err) {
                    tgState.value = 'error';
                    tgError.value = 'Не удалось получить статус';
                }
            }

            async function submitCode() {
                if (!authCode.value.trim()) return;
                submitting.value = true;
                try {
                    await API.post('/api/admin/telegram-code', { code: authCode.value.trim() });
                    authCode.value = '';
                    tgMessage.value = 'Код отправлен, ожидаем подтверждения...';
                    // Poll faster after submitting.
                    setTimeout(fetchTelegramStatus, 1000);
                    setTimeout(fetchTelegramStatus, 3000);
                } catch (err) {
                    tgError.value = err.message || 'Ошибка отправки кода';
                }
                submitting.value = false;
            }

            async function submitPassword() {
                if (!authPassword.value.trim()) return;
                submitting.value = true;
                try {
                    await API.post('/api/admin/telegram-password', { password: authPassword.value.trim() });
                    authPassword.value = '';
                    tgMessage.value = 'Пароль отправлен, ожидаем подтверждения...';
                    setTimeout(fetchTelegramStatus, 1000);
                    setTimeout(fetchTelegramStatus, 3000);
                } catch (err) {
                    tgError.value = err.message || 'Ошибка отправки пароля';
                }
                submitting.value = false;
            }

            function stateLabel(state) {
                var labels = {
                    'idle':             'Ожидание',
                    'pending_code':     'Ожидает код',
                    'pending_password': 'Ожидает пароль 2FA',
                    'authenticated':    'Авторизован',
                    'error':            'Ошибка',
                    'not_configured':   'Не настроен'
                };
                return labels[state] || state;
            }

            function stateColor(state) {
                if (state === 'authenticated') return 'var(--mp-green, #22c55e)';
                if (state === 'pending_code' || state === 'pending_password') return 'var(--mp-accent, #6c63ff)';
                if (state === 'error') return 'var(--mp-red, #ef4444)';
                return 'var(--mp-text-muted)';
            }

            onMounted(function () {
                fetchTelegramStatus();
                pollTimer = setInterval(fetchTelegramStatus, 3000);
            });

            onBeforeUnmount(function () {
                if (pollTimer) clearInterval(pollTimer);
            });

            return {
                tgState: tgState,
                tgMessage: tgMessage,
                tgError: tgError,
                authCode: authCode,
                authPassword: authPassword,
                submitting: submitting,
                submitCode: submitCode,
                submitPassword: submitPassword,
                stateLabel: stateLabel,
                stateColor: stateColor,
                fetchTelegramStatus: fetchTelegramStatus
            };
        },
        template: `
<div>
    <div class="mp-page-header">
        <h1><i class="bi bi-gear me-2"></i>Настройки</h1>
    </div>

    <div class="mp-page-content">
        <!-- Telegram Auth Section -->
        <div class="mp-settings-card">
            <div class="mp-settings-card__header">
                <i class="bi bi-telegram me-2" style="color:#26a5e4;"></i>
                Telegram MTProto
            </div>
            <div class="mp-settings-card__body">
                <!-- Status line -->
                <div class="d-flex align-items-center gap-2 mb-3">
                    <span class="mp-status-dot" :style="{ background: stateColor(tgState) }"></span>
                    <strong>{{ stateLabel(tgState) }}</strong>
                    <span v-if="tgMessage" class="text-muted ms-2" style="font-size:0.8rem;">
                        {{ tgMessage }}
                    </span>
                </div>

                <!-- Error alert -->
                <div v-if="tgError" class="alert alert-danger py-2 px-3" style="font-size:0.85rem;">
                    <i class="bi bi-exclamation-triangle me-1"></i>{{ tgError }}
                </div>

                <!-- Auth code input -->
                <div v-if="tgState === 'pending_code'" class="mp-auth-form">
                    <p style="font-size:0.85rem; color:var(--mp-text-secondary);">
                        Telegram отправил код авторизации в приложение. Введите его ниже:
                    </p>
                    <div class="input-group" style="max-width:320px;">
                        <input type="text" class="form-control"
                               v-model="authCode"
                               placeholder="12345"
                               maxlength="10"
                               @keyup.enter="submitCode"
                               :disabled="submitting"
                               autofocus>
                        <button class="btn btn-primary" @click="submitCode"
                                :disabled="submitting || !authCode.trim()">
                            <span v-if="submitting">
                                <span class="spinner-border spinner-border-sm me-1"></span>
                            </span>
                            <span v-else><i class="bi bi-send me-1"></i></span>
                            Отправить
                        </button>
                    </div>
                </div>

                <!-- 2FA password input -->
                <div v-if="tgState === 'pending_password'" class="mp-auth-form">
                    <p style="font-size:0.85rem; color:var(--mp-text-secondary);">
                        Требуется пароль двухфакторной авторизации Telegram:
                    </p>
                    <div class="input-group" style="max-width:320px;">
                        <input type="password" class="form-control"
                               v-model="authPassword"
                               placeholder="Пароль 2FA"
                               @keyup.enter="submitPassword"
                               :disabled="submitting"
                               autofocus>
                        <button class="btn btn-primary" @click="submitPassword"
                                :disabled="submitting || !authPassword.trim()">
                            <span v-if="submitting">
                                <span class="spinner-border spinner-border-sm me-1"></span>
                            </span>
                            <span v-else><i class="bi bi-shield-lock me-1"></i></span>
                            Отправить
                        </button>
                    </div>
                </div>

                <!-- Authenticated success message -->
                <div v-if="tgState === 'authenticated'" style="font-size:0.85rem; color:var(--mp-green);">
                    <i class="bi bi-check-circle me-1"></i>
                    Telegram подключен. Сессия сохранена — при следующем запуске код не потребуется.
                </div>

                <!-- Not configured -->
                <div v-if="tgState === 'not_configured'" style="font-size:0.85rem;">
                    <p class="text-muted mb-2">Для подключения установите переменные окружения:</p>
                    <code class="d-block mb-1">TELEGRAM_API_ID=ваш_api_id</code>
                    <code class="d-block mb-1">TELEGRAM_API_HASH="ваш_api_hash"</code>
                    <code class="d-block">TELEGRAM_PHONE="+7XXXXXXXXXX"</code>
                    <p class="text-muted mt-2" style="font-size:0.8rem;">
                        Получить API ID/Hash: <a href="https://my.telegram.org" target="_blank" rel="noopener">my.telegram.org</a>
                    </p>
                </div>

                <!-- Idle state -->
                <div v-if="tgState === 'idle'" style="font-size:0.85rem;" class="text-muted">
                    Telegram клиент ещё не запрашивал авторизацию. Запустите коллектор для начала работы.
                </div>

                <!-- Refresh button -->
                <div class="mt-3">
                    <button class="btn btn-sm btn-outline-secondary" @click="fetchTelegramStatus">
                        <i class="bi bi-arrow-clockwise me-1"></i>Обновить статус
                    </button>
                </div>
            </div>
        </div>

        <!-- Info card -->
        <div class="mp-settings-card mt-3">
            <div class="mp-settings-card__header">
                <i class="bi bi-info-circle me-2"></i>
                Как работает авторизация
            </div>
            <div class="mp-settings-card__body" style="font-size:0.85rem; color:var(--mp-text-secondary);">
                <ol class="mb-0" style="padding-left:1.2rem;">
                    <li>При первом запуске коллектора Telegram запрашивает код авторизации</li>
                    <li>Код приходит в приложение Telegram на телефон</li>
                    <li>Введите его на этой странице — код передаётся коллектору через Redis</li>
                    <li>После успешной авторизации сессия сохраняется в файл <code>data/tg.session</code></li>
                    <li>При последующих запусках код не потребуется (пока сессия не истечёт)</li>
                </ol>
            </div>
        </div>
    </div>
</div>
`
    };
})();
