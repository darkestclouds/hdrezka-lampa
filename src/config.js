// Версия и общие константы
const HDREZKA_VERSION = '0.1.0';

// Префикс логов (docs/ru/12-debug.md — «Соглашение по логированию»)
const LOG_PREFIX = '[HDRezka]';

// Имя Activity-компонента экрана выбора серий
const COMPONENT_NAME = 'hdrezka_online';

// Ключи Storage — все с префиксом плагина (docs/ru/04-storage-network.md, 11-pitfalls.md §6)
const STORAGE = {
    server: 'hdrezka_server',   // адрес своего API-сервера, напр. http://192.168.1.10:8765
    token: 'hdrezka_token',     // API-токен из config.json сервера
    choice: 'hdrezka_choice',   // { [movie.id]: { url, translator_id, season } }
    view: 'hdrezka_view'        // хэши просмотренных серий/фильмов
};

// Адрес сервера, если плагин загружен с него самого (http://host:port/hdrezka.js или /hdrezka.min.js).
// Вычисляется сразу при выполнении скрипта: document.currentScript доступен только в этот момент.
// Lampa подключает плагины через <script src> (utils/utils.js → putScriptAsync), поэтому он есть;
// запасной вариант — поиск своего <script> по имени файла.
const HDREZKA_SCRIPT_SERVER = (function () {
    var pattern = /^(https?:\/\/[^\/?#]+)\/hdrezka(?:\.min)?\.js(?:[?#].*)?$/i;

    try {
        var current = document.currentScript && document.currentScript.src;
        var match = current && current.match(pattern);
        if (match) return match[1];

        var scripts = document.getElementsByTagName('script');
        for (var i = scripts.length - 1; i >= 0; i--) {
            match = (scripts[i].src || '').match(pattern);
            if (match) return match[1];
        }
    } catch (e) {}

    return '';
})();

// API-токен, вшитый сервером при отдаче плагина (server/main.go → servePlugin).
// Сервер заменяет плейсхолдер на hex-строку токена; в файле из dist/ остаётся плейсхолдер (фигурные скобки
// не hex) — тогда токен берётся из настроек. Hex — чтобы любой токен безопасно лёг в JS-строку.
const HDREZKA_INJECTED_TOKEN = (function (hex) {
    if (!/^(?:[0-9a-f]{2})+$/.test(hex)) return '';
    try {
        return decodeURIComponent(hex.replace(/../g, '%$&'));
    } catch (e) {
        return '';
    }
})('{{HDREZKA_TOKEN}}');

// Иконка кнопки и раздела настроек
const PLUGIN_ICON = '<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><rect x="2" y="4" width="20" height="16" rx="3" stroke="currentColor" stroke-width="2"/><path d="M10 9.2v5.6c0 .4.4.6.7.4l4.5-2.8c.3-.2.3-.6 0-.8l-4.5-2.8c-.3-.2-.7 0-.7.4z" fill="currentColor"/></svg>';
