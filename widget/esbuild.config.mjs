import { build, transform } from 'esbuild';
import { readFile } from "fs/promises"
import inlineWorkerPlugin from 'esbuild-plugin-inline-worker';

const stage = process.env.STAGE || 'dev';
const buildTarget = process.env.BUILD_TARGET || 'default';

const config = {
  dev: {
    minify: false,
    sourcemap: true,
  },
  prod: {
    minify: true,
    sourcemap: true,
  }
};

let CSSMinifyPlugin = {
    name: "CSSMinifyPlugin",
    setup(build) {
        build.onLoad({ filter: /\.css$/ }, async (args) => {
            const f = await readFile(args.path)
            const css = await transform(f, { loader: "css", minify: true })
            return { loader: "text", contents: css.code }
        })
    }
}

let entryPointsConfig;
let outfileConfig;

if (buildTarget === 'library') {
  entryPointsConfig = ['./js/widget.js'];
  outfileConfig = './lib/index.js';
} else { // 'default'
  entryPointsConfig = ['./js/captcha.js'];
  outfileConfig = './static/js/privatecaptcha.js';
}

build({
    entryPoints: entryPointsConfig,
    bundle: true,
    outfile: outfileConfig,
    loader: { '.css': 'text' },
    plugins: [
        CSSMinifyPlugin,
        inlineWorkerPlugin({
            minify: config[stage].minify
        }),
    ],
    ...config[stage]
}).catch(() => process.exit(1));

