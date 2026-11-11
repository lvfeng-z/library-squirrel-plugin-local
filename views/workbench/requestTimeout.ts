// 工作台页请求助手的超时口径（预览面大库性能方案·决策2）：预览类动作
// （previewRule/summarizeRule/summarizeWorkSetRule）对识别快照全库合并求值，
// 耗时随库规模线性增长（大库分钟级），超时放宽至 180 秒——等待态沿用既有
// 「预览中…」交互零变化；其余动作维持 30 秒缺省。单一取值点供各页 request
// 助手共用（超时降级文案沿用既有「插件响应超时」口径，不含时长字样，零改动）。
export const DEFAULT_REQUEST_TIMEOUT_MS = 30000
export const PREVIEW_REQUEST_TIMEOUT_MS = 180000

// 预览类动作名单（与后端 workbenchAction* 的只读预览动作一一对应）。
const PREVIEW_REQUEST_ACTIONS = new Set(['previewRule', 'summarizeRule', 'summarizeWorkSetRule'])

// requestTimeoutMs 按动作名取请求超时：预览类 180 秒，其余动作 30 秒。
export function requestTimeoutMs(action: string): number {
  return PREVIEW_REQUEST_ACTIONS.has(action) ? PREVIEW_REQUEST_TIMEOUT_MS : DEFAULT_REQUEST_TIMEOUT_MS
}
