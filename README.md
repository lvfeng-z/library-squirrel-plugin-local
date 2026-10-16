# library-squirrel-plugin-local

[LibrarySquirrel](https://gitee.com/lv__feng/library-squirrel) 的本地导入插件（localImport）：从本地路径导入文件到资源库。

## 本地导入工作台（骨架期）

- **创建入口三分支**（`work_fetch.go` `CreateWithHeartbeat`）：宿主新建任务输本地路径后——库根命中既有解析会话→意图信号「转入解析」零任务收尾；无会话→Create 流内弹「开始解析（默认）/取消」确认弹窗（等待期经 `HeartbeatCreateFetcher` 心跳保活，插件侧 5 分钟超时=取消，两态均以意图信号收尾，零任务零落库）；工作台发起的创建（会话带发起标记）→过滤产出分支（选择集过滤属后续阶段）。
- **会话管理器**（`session.go`）：会话=按库根持久化工件（KV 键 `workbench-session:<库根标识>`，JSON、schemaVersion 自管），同库根至多一活动会话，跨重启恢复；档案写（泛化/裁决生成/规则编辑面三入口）与会话写一律经会话管理器单点串行；恢复会话时做规则档案对账（档案被外部变更则提示重新识别）。
- **前端事件协议**：`plugin:local-import:workbench:{request|response|event}` 族，请求带序号配对（`frontend/src/workbench/` 入口 + `views/workbench/` 组件，经既有 Vite 预编译管线构建为多页面产物）。
- **页面声明**（`plugin.json`，契约 18）：`workbench` 一级页面（会话列表）+ 侧栏菜单 + `workbench-parse` 二级解析页面（`content.parent`；双栏工作区——左栏识别全景清单与筛选，右栏解析面板 `ParsePanel.vue` 承载单次赋值/泛化成规则/冲突裁决）+ 其两个子页（规则管理、解析观测〔「建设中」挂载位〕）+ 入口确认弹窗 dialog。解析视图头部另设「规则管理」「解析观测」直链。

## 许可协议

本项目采用 [MIT](LICENSE) 许可协议。

Copyright (c) 2026 lvfeng
