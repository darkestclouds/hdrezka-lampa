/**
 * Сборка плагина HDRezka:
 *   node build.mjs          → dist/hdrezka.js + dist/hdrezka.min.js
 *   npm run build
 *   npm run build:watch
 */
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import * as esbuild from 'esbuild';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = __dirname;
const srcDir = path.join(root, 'src');
const distDir = path.join(root, 'dist');
const manifestPath = path.join(root, 'modules.json');

const BANNER = `/**
 * HDRezka — онлайн-источник для Lampa через свой сервер (server/)
 * Собран автоматически из src/. Не редактируйте этот файл вручную.
 */
`;

function readManifest() {
    return JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
}

function indentBlock(text, spaces = 4) {
    const pad = ' '.repeat(spaces);
    return text
        .replace(/\r\n/g, '\n')
        .replace(/\n+$/, '')
        .split('\n')
        .map((line) => (line.length ? pad + line : ''))
        .join('\n');
}

/** CSS внутри шаблонной строки esbuild не минифицирует — чистим сами */
function minifyCss(css) {
    return css
        .replace(/\r\n/g, '\n')
        .replace(/\/\*[\s\S]*?\*\//g, '')
        .replace(/\s+/g, ' ')
        // «+» не трогаем: в calc() пробелы вокруг + обязательны
        .replace(/\s*([{}:;,>~])\s*/g, '$1')
        .replace(/;}/g, '}')
        .trim();
}

function assemble(manifest, { compactCss = false } = {}) {
    const parts = [];

    for (const rel of manifest.modules) {
        const filePath = path.join(srcDir, rel);
        if (!fs.existsSync(filePath)) {
            throw new Error(`Модуль не найден: src/${rel}`);
        }

        let code = fs.readFileSync(filePath, 'utf8').replace(/\r\n/g, '\n');

        if (rel === 'styles/inject.js') {
            const cssPath = path.join(srcDir, manifest.css);
            if (!fs.existsSync(cssPath)) {
                throw new Error(`CSS не найден: src/${manifest.css}`);
            }
            let css = fs.readFileSync(cssPath, 'utf8').replace(/\r\n/g, '\n').replace(/\n+$/, '');
            if (compactCss) css = minifyCss(css);
            if (!code.includes(manifest.cssPlaceholder)) {
                throw new Error(`В styles/inject.js нет плейсхолдера ${manifest.cssPlaceholder}`);
            }
            code = code.split(manifest.cssPlaceholder).join(css);
        }

        parts.push(`    // ----- ${rel} -----\n${indentBlock(code)}`);
    }

    return (
        BANNER +
        '(function () {\n' +
        "    'use strict';\n\n" +
        parts.join('\n\n') +
        '\n\n})();\n'
    );
}

async function minifyJs(code) {
    // esbuild не чистит /* */ внутри строк (CSS в template literals) — вычищаем до minify
    const withoutCssComments = code.replace(/\/\*[\s\S]*?\*\//g, '');

    const result = await esbuild.transform(withoutCssComments, {
        minify: true,
        target: ['es2015'],
        charset: 'utf8',
        legalComments: 'none'
    });
    return `/*! HDRezka for Lampa | built from src/ | do not edit */\n` + result.code;
}

async function build() {
    const manifest = readManifest();
    fs.mkdirSync(distDir, { recursive: true });

    const normal = assemble(manifest);
    // Проверка синтаксиса обычной сборки (esbuild упадёт с понятной ошибкой)
    await esbuild.transform(normal, { target: ['es2015'], charset: 'utf8' });
    fs.writeFileSync(path.join(distDir, 'hdrezka.js'), normal, 'utf8');

    const min = await minifyJs(assemble(manifest, { compactCss: true }));
    fs.writeFileSync(path.join(distDir, 'hdrezka.min.js'), min, 'utf8');

    const kb = (n) => (Buffer.byteLength(n, 'utf8') / 1024).toFixed(1);
    console.log(`✓ dist/hdrezka.js      ${kb(normal)} KB`);
    console.log(`✓ dist/hdrezka.min.js  ${kb(min)} KB`);
}

const watch = process.argv.includes('--watch');

if (watch) {
    console.log('Watch: src/ + modules.json → rebuild…');
    await build();

    let timer = null;
    const schedule = () => {
        clearTimeout(timer);
        timer = setTimeout(async () => {
            try {
                await build();
            } catch (err) {
                console.error('Build error:', err.message || err);
            }
        }, 150);
    };

    fs.watch(srcDir, { recursive: true }, schedule);
    fs.watch(manifestPath, schedule);
} else {
    await build();
}
