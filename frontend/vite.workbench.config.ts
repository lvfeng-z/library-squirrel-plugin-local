import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { renameSync } from 'fs'
import { defineConfig, type Plugin } from 'vite'

/**
 * 工作台多页面构建（产出 views/workbench/ 下的预编译组件）：与 classify 单入口
 * 配置共用工厂函数包装形态——产物为
 *   export default function(__VUE__, __WAILS_RUNTIME__) { ... return Component; }
 *
 * 构建形态：一个页面 = 一次独立单入口构建（build-workbench.mjs 按 mode 循环调
 * 起，入口名即 mode）。不使用多入口单构建——共享模块（如 plugin-vue 的 export
 * helper）会被拆为携带相对导入的共享 chunk，import 语句进入工厂函数体后为非法
 * JS；单入口构建内无跨入口共享，一切内联，产物自包含。
 *
 * CSS 与 js 同名成对（<入口名>.css/<入口名>.js），不统一改名 style.css。
 */

// 工作台前端组件入口清单：入口名（=产物名）→ 入口模块
const WORKBENCH_ENTRIES: Record<string, string> = {
  workbench: 'src/workbench/entry-workbench.ts',
  parse: 'src/workbench/entry-parse.ts',
  rules: 'src/workbench/entry-rules.ts',
  observation: 'src/workbench/entry-observation.ts',
  'entry-confirm': 'src/workbench/entry-entry-confirm.ts'
}

function componentFactoryPlugin(): Plugin {
  return {
    name: 'component-factory-wrapper',
    enforce: 'post',
    generateBundle(_options, bundle) {
      for (const fileName of Object.keys(bundle)) {
        const chunk = bundle[fileName]
        if (chunk.type === 'chunk' && chunk.isEntry) {
          let code = chunk.code

          // 替换 vue 导入（import 的 as 需转为解构的冒号语法）
          code = code.replace(
            /import\s*\{([^}]+)}\s*from\s*['"]vue['"];?/g,
            (_, imports) => `const {${imports.replace(/\s+as\s+/g, ': ')}} = __VUE__;`
          )

          // 替换 @wailsio/runtime 导入
          code = code.replace(
            /import\s*\{([^}]+)}\s*from\s*['"]@wailsio\/runtime['"];?/g,
            (_, imports) => `const {${imports.replace(/\s+as\s+/g, ': ')}} = __WAILS_RUNTIME__;`
          )

          // 替换 export { X as default } 为 return
          code = code.replace(
            /export\s*\{\s*(\w+)\s+as\s+default\s*}\s*;?/,
            'return $1;'
          )
          // 替换 export default X 为 return
          code = code.replace(
            /^export\s+default\s+(\w+)\s*;?\s*$/m,
            'return $1;'
          )

          // 包装为工厂函数
          chunk.code = `export default function(__VUE__, __WAILS_RUNTIME__) {\n${code}\n}\n`
        }
      }
    },
    writeBundle(options, bundle) {
      // CSS 统一为本入口的 <入口名>.css（单入口构建只有一份 CSS 资产）
      const outDir = options.dir || resolve(__dirname, '../views/workbench')
      for (const fileName of Object.keys(bundle)) {
        if (fileName.endsWith('.css')) {
          const wanted = `${process.env.WORKBENCH_ENTRY}.css`
          if (fileName !== wanted) {
            try {
              renameSync(resolve(outDir, fileName), resolve(outDir, wanted))
            } catch {
              // 文件可能不存在
            }
          }
        }
      }
    }
  }
}

export default defineConfig(({ mode }) => {
  const entryPath = WORKBENCH_ENTRIES[mode]
  if (!entryPath) {
    throw new Error(
      `未知的工作台构建入口 "${mode}"，可用入口: ${Object.keys(WORKBENCH_ENTRIES).join(', ')}`
    )
  }
  // 入口名经环境变量传给 writeBundle 的 CSS 改名步骤（vite mode 与 env 同名可用）
  process.env.WORKBENCH_ENTRY = mode
  return {
    plugins: [vue(), componentFactoryPlugin()],
    build: {
      lib: {
        entry: resolve(__dirname, entryPath),
        formats: ['es'],
        fileName: () => `${mode}.js`
      },
      rollupOptions: {
        external: ['vue', '@wailsio/runtime']
      },
      outDir: resolve(__dirname, '../views/workbench'),
      emptyOutDir: false,
      cssCodeSplit: true,
      sourcemap: false,
      minify: true
    }
  }
})
