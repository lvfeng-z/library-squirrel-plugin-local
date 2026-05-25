package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

// PathType 路径语义类型
type PathType string

const (
	PathTypeAuthor   PathType = "author"
	PathTypeTag      PathType = "tag"
	PathTypeWorkName PathType = "workName"
	PathTypeWorkSet  PathType = "workSet"
	PathTypeSite     PathType = "site"
	PathTypeUnknown  PathType = "unknown"
)

// ClassifyQuestion 发送给前端的分类问题
type ClassifyQuestion struct {
	Level   int      `json:"level"`
	DirName string   `json:"dirName"`
	Options []string `json:"options"`
}

// ClassifyResponse 前端返回的分类响应
type ClassifyResponse struct {
	Level   int    `json:"level"`
	DirName string `json:"dirName"`
	Type    string `json:"type"`
	Cancel  bool   `json:"cancel,omitempty"`
}

// PathClassifier 路径分类器，管理已学规则并与前端交互
type PathClassifier struct {
	ctx          pluginsdk.PluginContext
	learnedRules map[int]PathType // level → type
	pendingCh    chan *ClassifyResponse
	mu           sync.Mutex
}

// NewPathClassifier 创建路径分类器
func NewPathClassifier(ctx pluginsdk.PluginContext) *PathClassifier {
	return &PathClassifier{
		ctx:          ctx,
		learnedRules: make(map[int]PathType),
		pendingCh:    make(chan *ClassifyResponse, 1),
	}
}

// HandleResponse 处理前端分类响应
func (c *PathClassifier) HandleResponse(resp *ClassifyResponse) {
	select {
	case c.pendingCh <- resp:
	default:
	}
}

// ClassifyDir 对目录名进行分类
func (c *PathClassifier) ClassifyDir(level int, dirName string) (PathType, error) {
	c.mu.Lock()
	if rule, ok := c.learnedRules[level]; ok {
		c.mu.Unlock()
		return rule, nil
	}
	c.mu.Unlock()

	// 询问前端 Slot
	question := &ClassifyQuestion{
		Level:   level,
		DirName: dirName,
		Options: []string{string(PathTypeAuthor), string(PathTypeTag), string(PathTypeWorkName), string(PathTypeWorkSet)},
	}
	data, err := json.Marshal(question)
	if err != nil {
		return PathTypeUnknown, err
	}

	if err := c.ctx.PublishToFrontend("plugin:local-import:classify:request", data); err != nil {
		c.ctx.Warnf("发送分类请求失败: %v，使用默认规则", err)
		return PathTypeWorkName, nil
	}

	// 等待用户响应
	select {
	case resp := <-c.pendingCh:
		if resp.Cancel {
			return PathTypeUnknown, fmt.Errorf("用户取消分类")
		}
		pt := PathType(resp.Type)
		c.mu.Lock()
		c.learnedRules[level] = pt
		c.mu.Unlock()
		return pt, nil
	case <-time.After(5 * time.Minute):
		c.ctx.Warnf("分类超时（5分钟），使用默认规则")
		return PathTypeWorkName, nil
	}
}
