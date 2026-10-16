// 工作台多页面构建编排：按入口清单逐入口调起 vite 单入口构建（配置见
// vite.workbench.config.ts——单入口构建保证产物自包含，无共享 chunk）。
import { build } from 'vite'

const entries = [
  'workbench',
  'parse',
  'rules',
  'observation',
  'entry-confirm'
]

for (const entry of entries) {
  process.stdout.write(`[workbench] building ${entry} ...\n`)
  await build({
    configFile: 'vite.workbench.config.ts',
    mode: entry,
    logLevel: 'warn'
  })
}
process.stdout.write('[workbench] all entries built\n')
