// HTML-шаблоны (docs/ru/05-templates-lang.md). Текст с сайта/TMDB вставляется через .text(), не через {плейсхолдеры}.
function addTemplates() {
    // Серия или фильм: кадр TMDB с номером серии поверх, название, оригинальное название, инфо, описание, таймлайн
    Lampa.Template.add('hdrezka_item', '<div class="hdrezka-item selector">' +
        '<div class="hdrezka-item__img hdrezka-item__img--wide">' +
            '<img alt="">' +
            '<div class="hdrezka-item__num"></div>' +
        '</div>' +
        '<div class="hdrezka-item__body">' +
            '<div class="hdrezka-item__head">' +
                '<div class="hdrezka-item__title"></div>' +
                '<div class="hdrezka-item__viewed hide"></div>' +
            '</div>' +
            '<div class="hdrezka-item__original hide"></div>' +
            '<div class="hdrezka-item__info"></div>' +
            '<div class="hdrezka-item__descr hide"></div>' +
            '<div class="hdrezka-item__timeline"></div>' +
        '</div>' +
    '</div>');

    // Результат поиска: постер с сайта, название, оригинальные названия, год/страна/жанр
    Lampa.Template.add('hdrezka_folder', '<div class="hdrezka-item hdrezka-item--folder selector">' +
        '<div class="hdrezka-item__img hdrezka-item__img--poster">' +
            '<img alt="">' +
        '</div>' +
        '<div class="hdrezka-item__body">' +
            '<div class="hdrezka-item__title"></div>' +
            '<div class="hdrezka-item__original hide"></div>' +
            '<div class="hdrezka-item__info"></div>' +
            '<div class="hdrezka-item__descr hide"></div>' +
        '</div>' +
    '</div>');

    Lampa.Template.add('hdrezka_message', '<div class="hdrezka-message">' +
        '<div class="hdrezka-message__title"></div>' +
        '<div class="hdrezka-message__text"></div>' +
    '</div>');
}
