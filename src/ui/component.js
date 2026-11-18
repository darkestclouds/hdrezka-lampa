// Экран HDRezka: поиск → перевод/сезон в фильтре → список серий → плеер.
// Устройство (Explorer + Filter + Scroll, контроллер 'content') повторяет онлайн-компонент ядра
// (lampa-source/plugins/online_prestige/component.js); контракт компонента — docs/ru/02-lifecycle.md.
function HdrezkaComponent(object) {
    var network = new Lampa.Reguest();
    var scroll = new Lampa.Scroll({ mask: true, over: true });
    var files = new Lampa.Explorer(object);
    var filter = new Lampa.Filter(object);
    var movie = object.movie || {};

    var last = null;
    var initialized = false;
    var destroyed = false;
    var images = [];

    var state = {
        url: '',
        info: null,
        translator: null,
        seasons: [],
        season: 0
    };

    // ---------- выбор пользователя (по карточке) ----------

    function getChoice() {
        var all = Lampa.Storage.cache(STORAGE.choice, 3000, {});
        return all[movie.id] || {};
    }

    function saveChoice(patch) {
        var all = Lampa.Storage.cache(STORAGE.choice, 3000, {});
        var current = all[movie.id] || {};
        Object.keys(patch).forEach(function (key) { current[key] = patch[key]; });
        all[movie.id] = current;
        Lampa.Storage.set(STORAGE.choice, all);
    }

    function clearChoice() {
        var all = Lampa.Storage.cache(STORAGE.choice, 3000, {});
        delete all[movie.id];
        Lampa.Storage.set(STORAGE.choice, all);
    }

    function movieYear() {
        return ((movie.release_date || movie.first_air_date || '') + '').slice(0, 4);
    }

    function originalTitle() {
        return movie.original_name || movie.original_title || movie.name || movie.title || '';
    }

    // Картинка в .hdrezka-item__img: показываем после загрузки, при ошибке остаётся пустой блок
    function loadImage(html, src) {
        var box = html.find('.hdrezka-item__img');
        var img = box.find('img')[0];
        if (!img || !src) return;

        img.onload = function () {
            box.addClass('hdrezka-item__img--loaded');
        };
        img.onerror = function () {
            img.onerror = null;
            img.removeAttribute('src');
        };
        img.src = src;
        images.push(img);
    }

    function clearImages() {
        images.forEach(function (img) {
            img.onload = null;
            img.onerror = null;
            img.removeAttribute('src');
        });
        images = [];
    }

    function tmdbImage(path) {
        return path ? Lampa.TMDB.image('t/p/w300' + path) : '';
    }

    function setText(html, selector, text) {
        var el = html.find(selector);
        el.text(text || '');
        el.toggleClass('hide', !text);
    }

    // ---------- состояние экрана ----------

    this.loading = function (status) {
        if (status) this.activity.loader(true);
        else {
            this.activity.loader(false);
            this.activity.toggle();
        }
    };

    this.reset = function () {
        last = null;
        clearImages();
        scroll.render().find('.empty').remove();
        scroll.clear();
    };

    this.message = function (title, text) {
        if (destroyed) return;
        this.reset();

        var html = Lampa.Template.get('hdrezka_message', {});
        html.find('.hdrezka-message__title').text(title);
        html.find('.hdrezka-message__text').text(text || '');
        scroll.append(html);

        this.loading(false);
    };

    this.fail = function (text) {
        this.message(Lampa.Lang.translate('hdrezka_title'), text);
    };

    // ---------- фильтр: перевод и сезон ----------

    this.buildFilter = function () {
        var select = [{ title: Lang('hdrezka_reset'), reset: true }];
        var chosen = [];
        var info = state.info;

        if (info && info.translators.length) {
            var current = state.translator;
            select.push({
                title: Lang('hdrezka_voice'),
                subtitle: current ? current.name : '',
                stype: 'voice',
                items: info.translators.map(function (t, i) {
                    return {
                        title: t.name + (t.premium ? ' · ' + Lang('hdrezka_premium') : ''),
                        selected: current && current.id === t.id,
                        index: i
                    };
                })
            });
            if (current) chosen.push(Lang('hdrezka_voice') + ': ' + current.name);
        }

        if (state.seasons.length) {
            select.push({
                title: Lang('hdrezka_season'),
                subtitle: Lang('hdrezka_season') + ' ' + state.season,
                stype: 'season',
                items: state.seasons.map(function (s, i) {
                    return {
                        title: Lang('hdrezka_season') + ' ' + s.season,
                        selected: s.season === state.season,
                        index: i
                    };
                })
            });
            chosen.push(Lang('hdrezka_season') + ': ' + state.season);
        }

        filter.set('filter', select);
        filter.chosen('filter', chosen);
    };

    this.closeFilter = function () {
        if ($('body').hasClass('selectbox--open')) Lampa.Select.close();
    };

    // ---------- поиск ----------

    this.initialize = function () {
        var _this = this;

        filter.onSearch = function (value) {
            Lampa.Activity.replace({ search: value, clarification: true });
        };
        filter.onBack = function () {
            _this.start();
        };
        filter.onSelect = function (type, a, b) {
            if (type !== 'filter') return;
            if (a.reset) {
                clearChoice();
                _this.closeFilter();
                state = { url: '', info: null, translator: null, seasons: [], season: 0 };
                object.clarification = false;
                _this.find();
                return;
            }
            if (a.stype === 'voice') _this.setVoice(b.index);
            if (a.stype === 'season') _this.setSeason(b.index);
            setTimeout(function () { _this.closeFilter(); }, 10);
        };

        if (filter.addButtonBack) filter.addButtonBack();

        files.appendFiles(scroll.render());
        files.appendHead(filter.render());
        scroll.body().addClass('torrent-list');
        scroll.minus(files.render().find('.explorer__files-head'));

        this.find();
    };

    this.find = function () {
        var _this = this;
        this.loading(true);
        this.buildFilter();

        if (!hdrezkaConfigured()) return this.fail(Lang('hdrezka_err_noserver'));

        var saved = getChoice();
        if (!object.clarification && saved.url) return this.open(saved.url);

        // Сервер сам ищет сначала «название год», потом без года; ручной запрос уходит как есть (raw)
        var params = object.clarification
            ? { title: object.search, raw: 1 }
            : {
                title: movie.title || movie.name || '',
                original_title: movie.original_title || movie.original_name || '',
                year: movieYear(),
                serial: movie.name ? 1 : 0
            };

        HdrezkaApi.search(network, params, function (json) {
            if (destroyed) return;
            var items = json.items || [];
            if (!items.length) return _this.fail(Lang('hdrezka_err_not_found'));

            // Автовыбор только при уверенном совпадении: название + год, без равного соперника
            var best = items[0];
            var sure = !object.clarification && best.score >= 90 && (items.length === 1 || items[1].score < best.score);

            if (sure) _this.open(best.url);
            else _this.showCandidates(items);
        }, function (text) {
            _this.fail(text);
        });
    };

    this.showCandidates = function (items) {
        var _this = this;
        this.reset();

        items.forEach(function (item) {
            var info = [];
            if (item.year) info.push(item.year);
            if (item.details) info.push(item.details);
            if (item.rating) info.push('★ ' + item.rating);

            var html = Lampa.Template.get('hdrezka_folder', {});
            html.find('.hdrezka-item__title').text(item.title);
            setText(html, '.hdrezka-item__original', (item.original_titles || []).join(' / '));
            html.find('.hdrezka-item__info').text(info.join(' · '));
            setText(html, '.hdrezka-item__descr', item.progress);
            loadImage(html, item.poster);

            html.on('hover:enter', function () {
                _this.open(item.url);
            }).on('hover:focus', function (e) {
                last = e.target;
                scroll.update($(e.target), true);
            });

            scroll.append(html);
        });

        this.loading(false);
    };

    // ---------- фильм / сериал ----------

    this.open = function (url) {
        var _this = this;
        this.loading(true);

        HdrezkaApi.info(network, url, function (info) {
            if (destroyed) return;

            state.url = url;
            state.info = info;
            state.seasons = [];
            saveChoice({ url: url });

            var saved = getChoice();
            var list = info.translators;
            state.translator =
                list.filter(function (t) { return t.id === saved.translator_id; })[0] ||
                list.filter(function (t) { return t.default && !t.premium; })[0] ||
                list.filter(function (t) { return !t.premium; })[0] ||
                list[0] || null;

            if (!state.translator) return _this.fail(Lang('hdrezka_err_no_stream'));

            _this.buildFilter();

            if (info.serial) _this.loadEpisodes();
            else _this.drawMovie();
        }, function (text) {
            _this.fail(text);
        });
    };

    this.setVoice = function (index) {
        var t = state.info && state.info.translators[index];
        if (!t) return;

        state.translator = t;
        saveChoice({ translator_id: t.id });
        this.buildFilter();

        if (state.info.serial) {
            this.loading(true);
            this.loadEpisodes();
        } else {
            this.drawMovie();
        }
    };

    this.setSeason = function (index) {
        var s = state.seasons[index];
        if (!s) return;

        state.season = s.season;
        saveChoice({ season: s.season });
        this.buildFilter();
        this.drawEpisodes();
    };

    this.loadEpisodes = function () {
        var _this = this;

        HdrezkaApi.episodes(network, state.url, state.translator.id, function (json) {
            if (destroyed) return;

            state.seasons = json.seasons || [];
            if (!state.seasons.length) return _this.fail(Lang('hdrezka_err_no_stream'));

            var saved = getChoice().season;
            var has = state.seasons.filter(function (s) { return s.season === saved; }).length;
            state.season = has ? saved : state.seasons[0].season;

            _this.buildFilter();
            _this.drawEpisodes();
        }, function (text) {
            _this.fail(text);
        });
    };

    this.drawMovie = function () {
        var title = movie.title || movie.name || state.info.title;
        var original = movie.original_title || movie.original_name || '';

        this.draw([{
            title: title,
            original: original && original !== title ? original : '',
            overview: movie.overview || '',
            image: tmdbImage(movie.backdrop_path),
            season: 0,
            episode: 0
        }]);
    };

    // Сезон из TMDB (как в онлайн-компоненте ядра). lang — язык запроса; пусто — язык интерфейса TMDB в Lampa.
    function tmdbSeason(number, lang, done) {
        var tmdb = Lampa.Api && Lampa.Api.sources && Lampa.Api.sources.tmdb;
        if (typeof movie.id !== 'number' || !movie.name || !tmdb) return done([]);

        tmdb.get('tv/' + movie.id + '/season/' + number, lang ? { langs: lang } : {}, function (data) {
            done((data && data.episodes) || []);
        }, function () {
            done([]);
        });
    }

    this.drawEpisodes = function () {
        var _this = this;
        var season = state.seasons.filter(function (s) { return s.season === state.season; })[0];
        if (!season) return;

        // Оригинальные названия — отдельным запросом на языке оригинала, если он отличается от языка интерфейса
        var uiLang = ((Lampa.Storage.field('tmdb_lang') || 'ru') + '').slice(0, 2);
        var origLang = (movie.original_language || '') + '';
        var needOriginal = origLang && origLang !== uiLang;

        var localized = null;
        var originals = needOriginal ? null : [];

        var build = function () {
            if (destroyed || localized === null || originals === null) return;

            _this.draw(season.episodes.map(function (ep) {
                var meta = localized.filter(function (e) { return e.episode_number === ep; })[0] || {};
                var orig = originals.filter(function (e) { return e.episode_number === ep; })[0] || {};
                var title = meta.name || Lang('hdrezka_episode') + ' ' + ep;
                // TMDB без перевода отдаёт «Episode 3» / «Серия 3» — такое оригиналом не считаем
                var original = orig.name && orig.name !== title && !/^(episode|эпизод|серия)\s*\d+$/i.test(orig.name) ? orig.name : '';

                return {
                    title: title,
                    original: original,
                    overview: meta.overview || orig.overview || '',
                    image: tmdbImage(meta.still_path || orig.still_path),
                    air_date: meta.air_date || '',
                    runtime: meta.runtime || 0,
                    season: season.season,
                    episode: ep
                };
            }));
        };

        tmdbSeason(season.season, '', function (list) {
            localized = list;
            build();
        });
        if (needOriginal) {
            tmdbSeason(season.season, origLang, function (list) {
                originals = list;
                build();
            });
        }
    };

    // ---------- список и плеер ----------

    this.draw = function (items) {
        var _this = this;
        var original = originalTitle();
        var viewed = Lampa.Storage.cache(STORAGE.view, 5000, []);
        var lastEpisode = getChoice().last_episode;
        var focusTarget = null;

        this.reset();

        items.forEach(function (element) {
            // Тот же хэш, что у ядра (interaction/timeline.js: watched / watchedEpisode) —
            // прогресс виден в карточке и в списках серий ядра
            var hash = element.season
                ? Lampa.Utils.hash([element.season, element.season > 10 ? ':' : '', element.episode, original].join(''))
                : Lampa.Utils.hash(original);

            element.view_hash = Lampa.Utils.hash([hash, state.translator.id].join(':'));
            element.timeline = Lampa.Timeline.view(hash, Lampa.Timeline.link(movie, element.season || undefined, element.episode || undefined));

            var info = [state.translator.name];
            if (element.air_date) info.push(Lampa.Utils.parseTime(element.air_date).full);
            if (element.runtime) info.push(Lampa.Utils.secondsToTime(element.runtime * 60, true));

            var html = Lampa.Template.get('hdrezka_item', {});
            html.find('.hdrezka-item__num').text(element.episode ? ('0' + element.episode).slice(-2) : '');
            html.find('.hdrezka-item__title').text(element.title);
            setText(html, '.hdrezka-item__original', element.original);
            html.find('.hdrezka-item__info').text(info.join(' · '));
            setText(html, '.hdrezka-item__descr', element.overview);
            html.find('.hdrezka-item__timeline').append(Lampa.Timeline.render(element.timeline));
            loadImage(html, element.image);

            element.mark = function () {
                viewed = Lampa.Storage.cache(STORAGE.view, 5000, []);
                if (viewed.indexOf(element.view_hash) === -1) {
                    viewed.push(element.view_hash);
                    Lampa.Storage.set(STORAGE.view, viewed);
                }
                html.find('.hdrezka-item__viewed').removeClass('hide');
                if (element.episode) saveChoice({ last_episode: element.season + ':' + element.episode });
            };

            if (viewed.indexOf(element.view_hash) !== -1) html.find('.hdrezka-item__viewed').removeClass('hide');
            if (lastEpisode && lastEpisode === element.season + ':' + element.episode) focusTarget = html[0];

            html.on('hover:enter', function () {
                _this.play(element, items);
            }).on('hover:focus', function (e) {
                last = e.target;
                scroll.update($(e.target), true);
            });

            scroll.append(html);
        });

        if (focusTarget) last = focusTarget;

        this.loading(false);
    };

    // Ссылки качеств по порядку сервера (лучшее первым) + выбор по настройке «Качество по умолчанию»
    function applyStream(element, json) {
        var quality = {};
        json.qualities.forEach(function (q) { quality[q.label] = q.mp4; });

        // Как в плеере ядра (getUrlQuality): совпадение по parseInt метки, иначе лучшее (сервер шлёт лучшее первым).
        // Сервер уже переименовал 2K/4K в 1440p/2160p, чтобы parseInt их понимал.
        var preferred = parseInt(Lampa.Storage.field('video_quality_default'), 10);
        var pick = json.qualities.filter(function (q) { return parseInt(q.label, 10) === preferred; })[0] || json.qualities[0];

        element.stream = pick.mp4;
        element.quality = quality;
        element.subtitles = json.subtitles.map(function (s) { return { label: s.label, url: s.url }; });
    }

    this.getStream = function (element, ok, fail) {
        if (element.stream) return ok(element);

        HdrezkaApi.stream(network, state.url, state.translator.id, element.season, element.episode, function (json) {
            if (!json.qualities || !json.qualities.length) return fail(Lang('hdrezka_err_no_stream'));
            applyStream(element, json);
            ok(element);
        }, fail);
    };

    this.play = function (element, items) {
        var _this = this;

        Lampa.Loading.start(function () {
            network.clear();
            Lampa.Loading.stop();
        });

        this.getStream(element, function () {
            Lampa.Loading.stop();
            if (destroyed) return;

            var first = {
                url: element.stream,
                quality: element.quality,
                title: element.episode ? element.title + ' · S' + element.season + 'E' + element.episode : element.title,
                subtitles: element.subtitles,
                timeline: element.timeline,
                card: movie
            };

            Lampa.Player.play(first);

            if (element.season) {
                // Плейлист сезона: ссылки остальных серий запрашиваются только при переходе к ним
                Lampa.Player.playlist(items.map(function (elem) {
                    var cell = {
                        title: elem.title + ' · S' + elem.season + 'E' + elem.episode,
                        timeline: elem.timeline,
                        card: movie
                    };

                    if (elem === element) {
                        cell.url = first.url;
                        cell.quality = first.quality;
                        cell.subtitles = first.subtitles;
                    } else {
                        cell.url = function (call) {
                            _this.getStream(elem, function () {
                                cell.url = elem.stream;
                                cell.quality = elem.quality;
                                cell.subtitles = elem.subtitles;
                                elem.mark();
                                call();
                            }, function () {
                                cell.url = '';
                                call();
                            });
                        };
                    }
                    return cell;
                }));
            } else {
                Lampa.Player.playlist([first]);
            }

            element.mark();
            if (movie.id) Lampa.Favorite.add('history', movie, 100);
        }, function (text) {
            Lampa.Loading.stop();
            Lampa.Noty.show(text, { style: 'error' });
        });
    };

    // ---------- контракт компонента ----------

    this.create = function () {
        return this.render();
    };

    this.start = function () {
        if (Lampa.Activity.active().activity !== this.activity) return;

        if (!initialized) {
            initialized = true;
            this.initialize();
        }

        Lampa.Background.immediately(Lampa.Utils.cardImgBackgroundBlur(movie));

        Lampa.Controller.add('content', {
            toggle: function () {
                Lampa.Controller.collectionSet(scroll.render(), files.render());
                Lampa.Controller.collectionFocus(last || false, scroll.render());
            },
            up: function () {
                if (Navigator.canmove('up')) Navigator.move('up');
                else Lampa.Controller.toggle('head');
            },
            down: function () {
                Navigator.move('down');
            },
            right: function () {
                if (Navigator.canmove('right')) Navigator.move('right');
                else filter.show(Lang('title_filter'), 'filter');
            },
            left: function () {
                if (Navigator.canmove('left')) Navigator.move('left');
                else Lampa.Controller.toggle('menu');
            },
            back: this.back.bind(this)
        });

        Lampa.Controller.toggle('content');
    };

    this.render = function () {
        return files.render();
    };

    this.back = function () {
        Lampa.Activity.backward();
    };

    this.pause = function () {};

    this.stop = function () {};

    this.destroy = function () {
        destroyed = true;
        clearImages();
        network.clear();
        files.destroy();
        scroll.destroy();
    };
}

function Lang(key) {
    return Lampa.Lang.translate(key);
}
