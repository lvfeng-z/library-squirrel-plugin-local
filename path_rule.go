package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// PathType 路径语义类型
type PathType string

const (
	PathTypeLocalAuthor PathType = "localAuthor"
	PathTypeSiteAuthor  PathType = "siteAuthor"
	PathTypeLocalTag    PathType = "localTag"
	PathTypeSiteTag     PathType = "siteTag"
	PathTypeWorkName    PathType = "workName"
	PathTypeWorkSet     PathType = "workSet"
	PathTypeSite        PathType = "site"
	PathTypeUnknown     PathType = "unknown"
)

// PathMeaning 单个路径段的含义
type PathMeaning struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ClassifyQuestion 发送给前端的分类问题
type ClassifyQuestion struct {
	Level   int      `json:"level"`
	DirName string   `json:"dirName"`
	Options []string `json:"options"`
}

// ClassifyResponse 前端返回的分类响应
type ClassifyResponse struct {
	Level    int           `json:"level"`
	DirName  string        `json:"dirName"`
	Meanings []PathMeaning `json:"meanings"`
	Cancel   bool          `json:"cancel,omitempty"`
}

// PathClassifier 路径分类器，管理已学规则并与前端交互
type PathClassifier struct {
	ctx          sdkdto.PluginContext
	learnedRules map[int][]string // level → 已学的类型列表
	pendingCh    chan *ClassifyResponse
	mu           sync.Mutex
}

// NewPathClassifier 创建路径分类器
func NewPathClassifier(ctx sdkdto.PluginContext) *PathClassifier {
	return &PathClassifier{
		ctx:          ctx,
		learnedRules: make(map[int][]string),
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

// ClassifyDir 对目录名进行分类，返回该目录的所有含义
func (c *PathClassifier) ClassifyDir(level int, dirName string) ([]PathMeaning, error) {
	c.mu.Lock()
	if types, ok := c.learnedRules[level]; ok {
		c.mu.Unlock()
		c.ctx.Infof("目录分类命中已学规则: level=%d, dirName=%s, types=%v", level, dirName, types)
		meanings := make([]PathMeaning, len(types))
		for i, t := range types {
			meanings[i] = PathMeaning{Type: t, Name: dirName}
		}
		return meanings, nil
	}
	c.mu.Unlock()

	c.ctx.Infof("开始目录分类询问: level=%d, dirName=%s", level, dirName)

	question := &ClassifyQuestion{
		Level:   level,
		DirName: dirName,
		Options: []string{
			string(PathTypeLocalAuthor), string(PathTypeSiteAuthor),
			string(PathTypeLocalTag), string(PathTypeSiteTag),
			string(PathTypeWorkName), string(PathTypeWorkSet),
			string(PathTypeSite), string(PathTypeUnknown),
		},
	}
	data, err := json.Marshal(question)
	if err != nil {
		return nil, err
	}

	if err := c.ctx.PublishToFrontend("plugin:local-import:classify:request", data); err != nil {
		c.ctx.Warnf("发送分类请求失败: %v，使用默认规则", err)
		return []PathMeaning{{Type: string(PathTypeWorkName), Name: dirName}}, nil
	}
	c.ctx.Infof("分类请求已发送到前端，等待响应...")

	select {
	case resp := <-c.pendingCh:
		if resp.Cancel {
			return nil, fmt.Errorf("用户取消分类")
		}
		types := make([]string, len(resp.Meanings))
		for i, m := range resp.Meanings {
			types[i] = m.Type
		}
		c.mu.Lock()
		c.learnedRules[level] = types
		c.mu.Unlock()
		c.ctx.Infof("收到分类响应: level=%d, dirName=%s, meanings=%+v", level, dirName, resp.Meanings)
		return resp.Meanings, nil
	case <-time.After(5 * time.Minute):
		c.ctx.Warnf("分类超时（5分钟），使用默认规则")
		return []PathMeaning{{Type: string(PathTypeWorkName), Name: dirName}}, nil
	}
}
