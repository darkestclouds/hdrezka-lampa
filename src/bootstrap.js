// Инициализация, манифест, старт
var pluginManifest = {
    type: 'video',
    version: HDREZKA_VERSION,
    name: 'HDRezka',
    description: 'Онлайн-просмотр с HDRezka через свой сервер',
    component: COMPONENT_NAME,
    // Пункт в контекстном меню карточки (docs/ru/09-manifest-menu.md)
    onContextMenu: function () {
        return { name: Lampa.Lang.translate('hdrezka_title'), description: '' };
    },
    onContextLauch: function (card) {
        openHdrezka(card);
    }
};

// Вызывается только после appready (docs/ru/01-getting-started.md «Защита appready»)
function initializePlugin() {
    console.log(LOG_PREFIX, 'v' + HDREZKA_VERSION);

    addTranslations();
    addTemplates();
    addStyles();

    Lampa.Component.add(COMPONENT_NAME, HdrezkaComponent);
    Lampa.Manifest.plugins = pluginManifest;

    addSettings();
    attachFullButton();
}

// Защита от двойной загрузки (docs/ru/01-getting-started.md, 11-pitfalls.md §1)
function startHdrezka() {
    window.hdrezka_plugin_ready = true;

    if (window.appready) {
        initializePlugin();
    } else {
        Lampa.Listener.follow('app', function (event) {
            if (event.type === 'ready') initializePlugin();
        });
    }
}

if (!window.hdrezka_plugin_ready) startHdrezka();
