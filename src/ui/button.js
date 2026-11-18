// Кнопка источника в полной карточке. Кладём её в скрытый .buttons--container рядом с торрентами —
// ядро само покажет её в списке «Смотреть» (components/full/start/buttons.js → onWatch),
// как и кнопку онлайн-плагина ядра (plugins/online_prestige/online_prestige.js).
function openHdrezka(card) {
    Lampa.Activity.push({
        url: '',
        title: Lampa.Lang.translate('hdrezka_title'),
        component: COMPONENT_NAME,
        search: card.title || card.name,
        search_one: card.title || card.name,
        search_two: card.original_title || card.original_name,
        movie: card,
        page: 1
    });
}

function attachFullButton() {
    Lampa.Listener.follow('full', function (e) {
        if (e.type !== 'complite' || !e.data || !e.data.movie) return;

        var render = e.object.activity.render();
        if (render.find('.hdrezka--button').length) return;

        var btn = $('<div class="full-start__button selector view--online hdrezka--button" data-subtitle="HDRezka v' + HDREZKA_VERSION + '">' +
            PLUGIN_ICON + '<span>' + Lampa.Lang.translate('hdrezka_title') + '</span></div>');

        btn.on('hover:enter', function () {
            openHdrezka(e.data.movie);
        });

        var torrent = render.find('.view--torrent');
        if (torrent.length) torrent.after(btn);
        else render.find('.full-start-new__buttons, .full-start__buttons').first().append(btn);
    });
}
