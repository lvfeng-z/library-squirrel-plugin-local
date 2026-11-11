import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { renameSync } from 'fs'
import { defineConfig, type Plugin } from 'vite'

/**
 * 将构建输出包装为工厂函数模式：
 * export default function(__VUE__, __WAILS_RUNTIME__) { ... return Component; }
 *
 * 将 import { ... } from 'vue' 替换为 const { ... } = __VUE__
 * 将 import { ... } from '@wailsio/runtime' 替换为 const { ... } = __WAILS_RUNTIME__
 */
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

          // 替换 export { X as default } 为 return（标识符须容 $ 前缀：minify 产物
          // 的导出中间变量可能命名为 $c 这类 $ 系短名，\w 不含 $ 会让替换漏过，
          // export 语句残留工厂函数体内成非法 ESM——2026-11-11 装载形态实案）
          code = code.replace(
            /export\s*\{\s*([$\w]+)\s+as\s+default\s*}\s*;?/,
            'return $1;'
          )
          // 替换 export default X 为 return（[$\w] 理由同上）
          code = code.replace(
            /^export\s+default\s+([$\w]+)\s*;?\s*$/m,
            'return $1;'
          )

          // 防回归闸：包装体内不得残留任何语句位置的 export（宿主以动态 import
          // 装载本产物，非法 ESM 在装载期才炸且难溯源——构建期硬失败优于运行期翻车）
          const leftover = code.match(/^[^\S\n]*export[\s{]/m)
          if (leftover) {
            throw new Error(
              `工厂包装后仍残留 export 语句（存在未被改写的导出形态）: ${JSON.stringify(leftover[0])}`
            )
          }

          // 包装为工厂函数
          chunk.code = `export default function(__VUE__, __WAILS_RUNTIME__) {\n${code}\n}\n`
        }
      }
    },
    writeBundle(options, bundle) {
      // CSS 文件统一命名为 style.css
      const outDir = options.dir || resolve(__dirname, '../views/classify')
      for (const fileName of Object.keys(bundle)) {
        if (fileName.endsWith('.css') && fileName !== 'style.css') {
          const oldPath = resolve(outDir, fileName)
          const newPath = resolve(outDir, 'style.css')
          try {
            renameSync(oldPath, newPath)
          } catch {
            // 文件可能不存在
          }
        }
      }
    }
  }
}

export default defineConfig({
  plugins: [vue(), componentFactoryPlugin()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/entry.ts'),
      formats: ['es'],
      fileName: () => 'classify-panel.js'
    },
    rollupOptions: {
      external: ['vue', '@wailsio/runtime']
    },
    outDir: resolve(__dirname, '../views/classify'),
    emptyOutDir: false,
    cssCodeSplit: false,
    sourcemap: false,
    minify: true
  }
})
