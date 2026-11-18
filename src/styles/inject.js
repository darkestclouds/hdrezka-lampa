// CSS вшивается сборкой вместо плейсхолдера; добавляется в body внутри init() (docs/ru/11-pitfalls.md §8)
function addStyles() {
    Lampa.Template.add('hdrezka_css', `<style>/* __INJECT_CSS__ */</style>`);
    $('body').append(Lampa.Template.get('hdrezka_css', {}, true));
}
