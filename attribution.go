package main

import (
	"path/filepath"
	"regexp"

	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// pixivPageWorkIDPattern pixiv 原图文件名的页级作品 ID 形态：{illustid}[-{hex}]?_p{页号}，
// 与 pixiv 插件解析原图 URL 文件名段的捕获组同构。本地保存的 pixiv 原图文件名与该形态
// 天然一致，命中即可产出与站点下载同键的页级作品 ID。锚定全匹配：带前后缀的形似文件名
// （如 master 规格名）不命中，按 local 归属回退。
var pixivPageWorkIDPattern = regexp.MustCompile(`^(\d+(?:-[0-9a-f]+)?_p\d+)$`)

// parsePixivPageWorkID 从文件名（不含扩展名）解析 pixiv 页级作品 ID；非该形态返回空串
func parsePixivPageWorkID(fileName string) string {
	m := pixivPageWorkIDPattern.FindStringSubmatch(fileName)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// workAttribution 单文件的作品归属裁决结果
type workAttribution struct {
	SiteKey    string // 归属站点键（local 回退 = identity.Local.Key）
	PageWorkID string // 真实域页级作品 ID；空 = 以文件内容哈希为作品身份（local 回退）
}

// isReal 是否真实站点域归属（非 local 回退）
func (a workAttribution) isReal() bool {
	return a.PageWorkID != ""
}

// resolveWorkAttribution 裁决单文件的作品归属域：
//   - 站点分类含义（classifiedSiteKey，子树级声明）提供归属站点键；显式选择 local 站点
//     = 声明走本地域，压制文件名形态推断。
//   - pixiv 原图文件名形态提供页级作品 ID；无站点分类时该形态同时锚定归属站点 = pixiv
//     （该 ID 形态即 pixiv 注册表约定的站点侧作品 ID 形态）。
//   - 真实域归属必须携带解析出的页级作品 ID——本地造 ID（内容哈希）永不进真实域，
//     站点分类在场但文件名解析不出真实 ID 时回退 local。
func resolveWorkAttribution(fileName string, classifiedSiteKey string) workAttribution {
	pageID := parsePixivPageWorkID(stripExt(fileName))
	if classifiedSiteKey != "" && classifiedSiteKey != identity.Local.Key {
		if pageID != "" {
			return workAttribution{SiteKey: classifiedSiteKey, PageWorkID: pageID}
		}
	} else if classifiedSiteKey == "" && pageID != "" {
		return workAttribution{SiteKey: identity.Pixiv.Key, PageWorkID: pageID}
	}
	return workAttribution{SiteKey: identity.Local.Key}
}

// resolveClassifiedSiteKey 从路径含义集合中取子树站点键：site 类型含义的 ID 为站点 DB 行 id
// （面板站点下拉选择项的 value），经 resolveSiteKeyByID 解析为站点键；多个 site 含义时
// 最深层（元数据序最靠后）生效。无可解析的 site 含义时返回空串。
func resolveClassifiedSiteKey(metadata []PathMeaning, resolveSiteKeyByID func(string) string) string {
	key := ""
	for _, m := range metadata {
		if m.Type != string(PathTypeSite) {
			continue
		}
		if k := resolveSiteKeyByID(m.ID); k != "" {
			key = k
		}
	}
	return key
}

// attributedFile 带归属裁决的扫描文件
type attributedFile struct {
	entry FileEntry
	attr  workAttribution
}

// partitionByAttribution 把同目录文件按归属裁决拆为 local 回退组与真实域组。
// 同目录文件共享同一份路径含义（站点分类一致），真实域组内站点键恒一致。
func partitionByAttribution(files []FileEntry, classifiedSiteKey string) (local, real []attributedFile) {
	for _, f := range files {
		attr := resolveWorkAttribution(filepath.Base(f.FullPath), classifiedSiteKey)
		af := attributedFile{entry: f, attr: attr}
		if attr.isReal() {
			real = append(real, af)
		} else {
			local = append(local, af)
		}
	}
	return local, real
}
