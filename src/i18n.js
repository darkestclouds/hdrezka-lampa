// Строки интерфейса (docs/ru/05-templates-lang.md)
function addTranslations() {
    Lampa.Lang.add({
        hdrezka_title: { ru: 'HDRezka', en: 'HDRezka', uk: 'HDRezka' },
        hdrezka_settings_server: { ru: 'Адрес сервера', en: 'Server address', uk: 'Адреса сервера' },
        hdrezka_settings_server_descr: {
            ru: 'Ваш сервер hdrezka-server, например http://192.168.1.10:8765. Если плагин установлен с сервера, адрес подставится сам',
            en: 'Your hdrezka-server, e.g. http://192.168.1.10:8765. Filled in automatically when the plugin is installed from the server',
            uk: 'Ваш сервер hdrezka-server, наприклад http://192.168.1.10:8765. Якщо плагін встановлено із сервера, адреса підставиться сама'
        },
        hdrezka_settings_token: { ru: 'API-токен', en: 'API token', uk: 'API-токен' },
        hdrezka_settings_token_descr: {
            ru: 'Значение api_token из config.json сервера. Если плагин установлен с сервера, токен уже вшит',
            en: 'The api_token value from the server config.json. Already built in when the plugin is installed from the server',
            uk: 'Значення api_token з config.json сервера. Якщо плагін встановлено із сервера, токен уже вшито'
        },
        hdrezka_settings_account: { ru: 'Аккаунт HDRezka', en: 'HDRezka account', uk: 'Акаунт HDRezka' },
        hdrezka_settings_check: { ru: 'Проверить подключение', en: 'Check connection', uk: 'Перевірити підключення' },
        hdrezka_settings_login: { ru: 'Войти', en: 'Sign in', uk: 'Увійти' },
        hdrezka_settings_login_descr: {
            ru: 'Логин и пароль уходят только на ваш сервер, пароль не сохраняется',
            en: 'Login and password go only to your server; the password is not stored',
            uk: 'Логін і пароль йдуть лише на ваш сервер, пароль не зберігається'
        },
        hdrezka_settings_cookies: { ru: 'Вставить куки', en: 'Paste cookies', uk: 'Вставити куки' },
        hdrezka_settings_cookies_descr: {
            ru: 'Строка кук из браузера, где вы вошли на сайт: dle_user_id=…; dle_password=…',
            en: 'Cookie string from a browser signed in to the site: dle_user_id=…; dle_password=…',
            uk: 'Рядок кук із браузера, де ви увійшли на сайт: dle_user_id=…; dle_password=…'
        },
        hdrezka_settings_logout: { ru: 'Выйти', en: 'Sign out', uk: 'Вийти' },
        hdrezka_input_login: { ru: 'Логин или e-mail', en: 'Login or e-mail', uk: 'Логін або e-mail' },
        hdrezka_input_password: { ru: 'Пароль', en: 'Password', uk: 'Пароль' },
        hdrezka_status_ok: { ru: 'Сервер доступен', en: 'Server is reachable', uk: 'Сервер доступний' },
        hdrezka_status_logged: { ru: 'вход выполнен', en: 'signed in', uk: 'вхід виконано' },
        hdrezka_status_anon: { ru: 'без входа', en: 'not signed in', uk: 'без входу' },
        hdrezka_status_premium: { ru: 'премиум', en: 'premium', uk: 'преміум' },
        hdrezka_login_ok: { ru: 'Вход выполнен', en: 'Signed in', uk: 'Вхід виконано' },
        hdrezka_cookies_ok: { ru: 'Куки сохранены на сервере', en: 'Cookies saved on the server', uk: 'Куки збережено на сервері' },
        hdrezka_logout_ok: { ru: 'Сессия удалена', en: 'Session removed', uk: 'Сесію видалено' },

        hdrezka_err_noserver: {
            ru: 'Укажите адрес сервера и токен в настройках HDRezka',
            en: 'Set the server address and token in HDRezka settings',
            uk: 'Вкажіть адресу сервера і токен у налаштуваннях HDRezka'
        },
        hdrezka_err_unauthorized: { ru: 'Неверный API-токен', en: 'Wrong API token', uk: 'Невірний API-токен' },
        hdrezka_err_network: { ru: 'Сервер HDRezka не отвечает', en: 'HDRezka server is not responding', uk: 'Сервер HDRezka не відповідає' },
        hdrezka_err_no_mirror: {
            ru: 'На сервере не задано зеркало HDRezka — укажите его в веб-интерфейсе сервера',
            en: 'No HDRezka mirror set on the server — set it in the server web interface',
            uk: 'На сервері не задано дзеркало HDRezka — вкажіть його у веб-інтерфейсі сервера'
        },
        hdrezka_err_unavailable: { ru: 'Сайт HDRezka недоступен с сервера', en: 'HDRezka site is unreachable from the server', uk: 'Сайт HDRezka недоступний із сервера' },
        hdrezka_err_login_required: { ru: 'Нужно войти в аккаунт HDRezka', en: 'HDRezka sign-in required', uk: 'Потрібно увійти в акаунт HDRezka' },
        hdrezka_err_login_failed: { ru: 'Не удалось войти', en: 'Sign-in failed', uk: 'Не вдалося увійти' },
        hdrezka_err_no_stream: { ru: 'Нет ссылки на видео', en: 'No video link', uk: 'Немає посилання на відео' },
        hdrezka_err_not_found: { ru: 'Ничего не найдено', en: 'Nothing found', uk: 'Нічого не знайдено' },

        hdrezka_voice: { ru: 'Перевод', en: 'Translation', uk: 'Переклад' },
        hdrezka_season: { ru: 'Сезон', en: 'Season', uk: 'Сезон' },
        hdrezka_episode: { ru: 'Серия', en: 'Episode', uk: 'Серія' },
        hdrezka_premium: { ru: 'премиум', en: 'premium', uk: 'преміум' },
        hdrezka_reset: { ru: 'Сбросить выбор', en: 'Reset choice', uk: 'Скинути вибір' },
        hdrezka_choose: { ru: 'Выберите, что смотреть', en: 'Choose what to watch', uk: 'Оберіть, що дивитися' }
    });
}
