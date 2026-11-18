// Клиент своего API-сервера (server/). Сеть — только через Lampa.Reguest (docs/ru/04-storage-network.md).
// Каждый вызов принимает экземпляр Lampa.Reguest вызывающего компонента, чтобы destroy() → network.clear()
// отменял колбэки незавершённых запросов.

// Введённый вручную адрес, иначе — сервер, с которого загружен плагин (default параметра в settings.js)
function hdrezkaServer() {
    var url = (Lampa.Storage.get(STORAGE.server, HDREZKA_SCRIPT_SERVER) + '').trim() || HDREZKA_SCRIPT_SERVER;
    if (url && !/^https?:\/\//i.test(url)) url = 'http://' + url;
    return url.replace(/\/+$/, '');
}

// Введённый вручную токен, иначе — вшитый сервером при отдаче плагина
function hdrezkaToken() {
    return (Lampa.Storage.get(STORAGE.token, HDREZKA_INJECTED_TOKEN) + '').trim() || HDREZKA_INJECTED_TOKEN;
}

function hdrezkaConfigured() {
    return Boolean(hdrezkaServer() && hdrezkaToken());
}

function hdrezkaQuery(params) {
    var parts = [];
    Object.keys(params || {}).forEach(function (key) {
        var value = params[key];
        if (value === undefined || value === null || value === '') return;
        parts.push(encodeURIComponent(key) + '=' + encodeURIComponent(value));
    });
    return parts.length ? '?' + parts.join('&') : '';
}

// Текст ошибки для пользователя: по коду сервера, иначе сообщение сервера, иначе текст Lampa
function hdrezkaErrorText(network, xhr, exception) {
    var json = xhr && xhr.responseJSON;
    if (!json && xhr && xhr.responseText) {
        try { json = JSON.parse(xhr.responseText); } catch (e) { json = null; }
    }

    if (json && json.error) {
        var key = 'hdrezka_err_' + json.error;
        var text = Lampa.Lang.translate(key);
        if (text === key) return json.message || json.error;
        // для «нет ссылки» и неудачного входа полезно показать и причину с сайта
        if ((json.error === 'no_stream' || json.error === 'login_failed') && json.message) text += ': ' + json.message;
        return text;
    }
    if (xhr && xhr.status === 0 && exception !== 'timeout') return Lampa.Lang.translate('hdrezka_err_network');
    return network.errorDecode(xhr, exception);
}

/**
 * Запрос к серверу.
 * @param {Lampa.Reguest} network
 * @param {string} path      '/api/…'
 * @param {object} query     параметры строки запроса
 * @param {object|null} post данные POST (form-urlencoded)
 * @param {function} onSuccess (json)
 * @param {function} onError   (text, code)
 */
function hdrezkaRequest(network, path, query, post, onSuccess, onError) {
    if (!hdrezkaConfigured()) {
        onError(Lampa.Lang.translate('hdrezka_err_noserver'), 'noserver');
        return;
    }

    network.silent(
        hdrezkaServer() + path + hdrezkaQuery(query),
        function (json) {
            onSuccess(json);
        },
        function (xhr, exception) {
            var code = xhr && xhr.responseJSON ? xhr.responseJSON.error : '';
            onError(hdrezkaErrorText(network, xhr, exception), code);
        },
        post || false,
        {
            dataType: 'json',
            timeout: 1000 * 40, // первый запрос может решать анти-бот и перебирать зеркала
            headers: { 'X-Api-Key': hdrezkaToken() }
        }
    );
}

var HdrezkaApi = {
    status: function (network, check, ok, fail) {
        hdrezkaRequest(network, '/api/status', { check: check ? 1 : '' }, null, ok, fail);
    },
    login: function (network, login, password, ok, fail) {
        hdrezkaRequest(network, '/api/login', null, { login: login, password: password }, ok, fail);
    },
    cookies: function (network, cookies, ok, fail) {
        hdrezkaRequest(network, '/api/cookies', null, { cookies: cookies }, ok, fail);
    },
    logout: function (network, ok, fail) {
        hdrezkaRequest(network, '/api/logout', null, { logout: 1 }, ok, fail);
    },
    search: function (network, params, ok, fail) {
        hdrezkaRequest(network, '/api/search', params, null, ok, fail);
    },
    info: function (network, url, ok, fail) {
        hdrezkaRequest(network, '/api/info', { url: url }, null, ok, fail);
    },
    episodes: function (network, url, translatorId, ok, fail) {
        hdrezkaRequest(network, '/api/episodes', { url: url, translator_id: translatorId }, null, ok, fail);
    },
    stream: function (network, url, translatorId, season, episode, ok, fail) {
        hdrezkaRequest(network, '/api/stream', {
            url: url,
            translator_id: translatorId,
            season: season || '',
            episode: episode || ''
        }, null, ok, fail);
    }
};
