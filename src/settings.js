// Раздел настроек (docs/ru/08-settings.md)
var settingsNetwork = null;

function settingsNet() {
    if (!settingsNetwork) settingsNetwork = new Lampa.Reguest();
    return settingsNetwork;
}

function notify(text, error) {
    Lampa.Noty.show(text, error ? { style: 'error' } : undefined);
}

function askInput(title, password, done) {
    Lampa.Input.edit({
        title: title,
        value: '',
        free: true,
        nosave: true,
        password: password
    }, function (value) {
        value = (value || '') + '';
        if (value.trim()) done(value);
    });
}

function checkConnection() {
    HdrezkaApi.status(settingsNet(), true, function (st) {
        var parts = [st.logged_in ? Lampa.Lang.translate('hdrezka_status_logged') : Lampa.Lang.translate('hdrezka_status_anon')];
        if (st.premium) parts.push(Lampa.Lang.translate('hdrezka_status_premium'));
        notify(Lampa.Lang.translate('hdrezka_status_ok') + ' (' + st.mirror + '): ' + parts.join(', '));
    }, function (text) {
        notify(text, true);
    });
}

function login() {
    askInput(Lampa.Lang.translate('hdrezka_input_login'), false, function (name) {
        // Второе поле открываем после закрытия первого — Input.edit возвращает фокус в настройки
        setTimeout(function () {
            askInput(Lampa.Lang.translate('hdrezka_input_password'), true, function (password) {
                HdrezkaApi.login(settingsNet(), name.trim(), password, function () {
                    notify(Lampa.Lang.translate('hdrezka_login_ok'));
                }, function (text) {
                    notify(text, true);
                });
            });
        }, 50);
    });
}

function pasteCookies() {
    askInput(Lampa.Lang.translate('hdrezka_settings_cookies'), false, function (cookies) {
        HdrezkaApi.cookies(settingsNet(), cookies.trim(), function () {
            notify(Lampa.Lang.translate('hdrezka_cookies_ok'));
        }, function (text) {
            notify(text, true);
        });
    });
}

function logout() {
    HdrezkaApi.logout(settingsNet(), function () {
        notify(Lampa.Lang.translate('hdrezka_logout_ok'));
    }, function (text) {
        notify(text, true);
    });
}

function addSettings() {
    Lampa.SettingsApi.addComponent({
        component: 'hdrezka',
        name: Lampa.Lang.translate('hdrezka_title'),
        icon: PLUGIN_ICON
    });

    Lampa.SettingsApi.addParam({
        component: 'hdrezka',
        // Если плагин загружен с сервера — его адрес подставляется по умолчанию; ручной ввод главнее
        // values: '' обязателен для input — Params.update (interaction/settings/params.js) читает values[name]
        // как строку; без него падает с «Cannot read properties of undefined»
        param: { name: STORAGE.server, type: 'input', values: '', default: HDREZKA_SCRIPT_SERVER, placeholder: 'http://192.168.1.10:8765' },
        field: {
            name: Lampa.Lang.translate('hdrezka_settings_server'),
            description: Lampa.Lang.translate('hdrezka_settings_server_descr')
        }
    });

    Lampa.SettingsApi.addParam({
        component: 'hdrezka',
        // Плагин с сервера приходит с уже вшитым токеном; поле нужно, только если плагин подключён из другого места
        param: { name: STORAGE.token, type: 'input', values: '', default: HDREZKA_INJECTED_TOKEN, placeholder: '' },
        field: {
            name: Lampa.Lang.translate('hdrezka_settings_token'),
            description: Lampa.Lang.translate('hdrezka_settings_token_descr')
        }
    });

    Lampa.SettingsApi.addParam({
        component: 'hdrezka',
        param: { name: 'hdrezka_check', type: 'button' },
        field: { name: Lampa.Lang.translate('hdrezka_settings_check') },
        onChange: checkConnection
    });

    Lampa.SettingsApi.addParam({
        component: 'hdrezka',
        param: { name: 'hdrezka_account_title', type: 'title' },
        field: { name: Lampa.Lang.translate('hdrezka_settings_account') }
    });

    Lampa.SettingsApi.addParam({
        component: 'hdrezka',
        param: { name: 'hdrezka_login', type: 'button' },
        field: {
            name: Lampa.Lang.translate('hdrezka_settings_login'),
            description: Lampa.Lang.translate('hdrezka_settings_login_descr')
        },
        onChange: login
    });

    Lampa.SettingsApi.addParam({
        component: 'hdrezka',
        param: { name: 'hdrezka_cookies', type: 'button' },
        field: {
            name: Lampa.Lang.translate('hdrezka_settings_cookies'),
            description: Lampa.Lang.translate('hdrezka_settings_cookies_descr')
        },
        onChange: pasteCookies
    });

    Lampa.SettingsApi.addParam({
        component: 'hdrezka',
        param: { name: 'hdrezka_logout', type: 'button' },
        field: { name: Lampa.Lang.translate('hdrezka_settings_logout') },
        onChange: logout
    });
}
