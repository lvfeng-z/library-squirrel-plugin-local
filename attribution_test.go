package main

import (
	"path/filepath"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// TestParsePixivPageWorkID pixiv 原图文件名页级 ID 解析：命中形态原样产出 ID，
// 形似非原图（无 illust id/无页号/带前后缀/大写十六进制）一律不命中（回退 local 的前置）
func TestParsePixivPageWorkID(t *testing.T) {
	hits := []string{
		"12345678_p0",
		"12345678_p12",
		"12345678-abcdef_p0",
		"12345678-0123456789abcdef_p3",
	}
	for _, name := range hits {
		if got := parsePixivPageWorkID(name); got != name {
			t.Errorf("文件名 %q 应命中页级 ID，实得 %q", name, got)
		}
	}

	misses := []string{
		"",
		"myart_p0",               // 无 illust id 前缀数字
		"12345678",               // 无页号段
		"12345678_p0_master1200", // 形似非原图：master 规格名
		"img_12345678_p0",        // 带前缀非全匹配
		"12345678-ABCDEF_p0",     // 十六进制段大写不匹配
		"12345678_pabc",          // 页号非数字
		"12345678_p",             // 页号缺失
	}
	for _, name := range misses {
		if got := parsePixivPageWorkID(name); got != "" {
			t.Errorf("文件名 %q 不应命中页级 ID，实得 %q", name, got)
		}
	}
}

// TestResolveWorkAttribution 归属裁决分轨：站点分类提供站点键、pixiv 原图形态提供页级 ID，
// 真实域必须同时有站点键与页级 ID（本地造 ID 永不进真实域），显式 local 分类压制形态推断
func TestResolveWorkAttribution(t *testing.T) {
	cases := []struct {
		name       string
		fileName   string
		classified string
		wantSite   string
		wantPageID string
	}{
		{"无分类+原图形态→pixiv", "12345678-abcdef_p0.jpg", "", identity.Pixiv.Key, "12345678-abcdef_p0"},
		{"无分类+普通文件名→local", "度假照片.png", "", identity.Local.Key, ""},
		{"分类真实站点+原图形态→分类站点", "12345678_p0.png", identity.Bilibili.Key, identity.Bilibili.Key, "12345678_p0"},
		{"分类真实站点+普通文件名→local", "随手存图.png", identity.Bilibili.Key, identity.Local.Key, ""},
		{"显式分类local+原图形态→local", "12345678_p0.jpg", identity.Local.Key, identity.Local.Key, ""},
	}
	for _, c := range cases {
		got := resolveWorkAttribution(c.fileName, c.classified)
		if got.SiteKey != c.wantSite || got.PageWorkID != c.wantPageID {
			t.Errorf("%s: resolveWorkAttribution(%q, %q) = (%q, %q), 期望 (%q, %q)",
				c.name, c.fileName, c.classified, got.SiteKey, got.PageWorkID, c.wantSite, c.wantPageID)
		}
	}
}

// TestResolveClassifiedSiteKey site 含义 → 子树站点键：可解析 ID 生效、多个取最深层、
// 不可解析（空 ID/查无行）不影响其余含义
func TestResolveClassifiedSiteKey(t *testing.T) {
	resolver := func(id string) string {
		switch id {
		case "7":
			return identity.Pixiv.Key
		case "9":
			return identity.Bilibili.Key
		default:
			return ""
		}
	}

	cases := []struct {
		name     string
		metadata []PathMeaning
		want     string
	}{
		{"无site含义", []PathMeaning{{Type: "localTag", Name: "x"}}, ""},
		{"单个site含义可解析", []PathMeaning{{Type: "site", ID: "7", Name: "pixiv"}}, identity.Pixiv.Key},
		{"多个site含义取最深层", []PathMeaning{
			{Type: "site", ID: "7", Name: "pixiv"},
			{Type: "localTag", Name: "隔层"},
			{Type: "site", ID: "9", Name: "bilibili"},
		}, identity.Bilibili.Key},
		{"site含义ID查无行→无站点分类", []PathMeaning{{Type: "site", ID: "404", Name: "ghost"}}, ""},
		{"site含义无ID→无站点分类", []PathMeaning{{Type: "site", Name: "pixiv"}}, ""},
	}
	for _, c := range cases {
		if got := resolveClassifiedSiteKey(c.metadata, resolver); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestPartitionByAttribution 同目录文件按归属裁决拆分：无分类时仅原图形态文件进真实域；
// 分类在场时按分类站点裁决，解析不出真实 ID 的文件回退 local
func TestPartitionByAttribution(t *testing.T) {
	dir := t.TempDir()
	pixivForm := FileEntry{FullPath: filepath.Join(dir, "12345678_p0.jpg"), RelPath: "12345678_p0.jpg", Hash: "h1"}
	plain := FileEntry{FullPath: filepath.Join(dir, "photo.png"), RelPath: "photo.png", Hash: "h2"}

	local, real := partitionByAttribution([]FileEntry{pixivForm, plain}, "")
	if len(real) != 1 || real[0].attr.SiteKey != identity.Pixiv.Key || real[0].attr.PageWorkID != "12345678_p0" {
		t.Fatalf("无分类时应仅原图形态文件进真实域(pixiv)，实得 %+v", real)
	}
	if len(local) != 1 || local[0].entry.Hash != "h2" || local[0].attr.isReal() {
		t.Fatalf("无分类时普通文件应回退 local，实得 %+v", local)
	}

	local, real = partitionByAttribution([]FileEntry{pixivForm, plain}, identity.Bilibili.Key)
	if len(real) != 1 || real[0].attr.SiteKey != identity.Bilibili.Key {
		t.Fatalf("分类 bilibili 时原图形态文件应归 bilibili，实得 %+v", real)
	}
	if len(local) != 1 || local[0].attr.SiteKey != identity.Local.Key {
		t.Fatalf("分类 bilibili 时普通文件应回退 local，实得 %+v", local)
	}
}

// TestResolveSiteKeyByIDNilContext 无宿主上下文（ctx 未注入）时站点 DB 行 id 一律不可解析，
// 走无站点分类→local 回退
func TestResolveSiteKeyByIDNilContext(t *testing.T) {
	h := &LocalImportTaskHandler{}
	if got := h.resolveSiteKeyByID("7"); got != "" {
		t.Fatalf("nil ctx 下应返回空串，实得 %q", got)
	}
	if got := h.resolveSiteKeyByID("abc"); got != "" {
		t.Fatalf("非数字 id 应返回空串，实得 %q", got)
	}
}
