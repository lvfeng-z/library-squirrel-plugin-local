// 规则档案存取层的单元测试：内存 fake 存储的存读往返、空档案与损坏数据的降级、
// 坏规则逐条跳过（错误携带规则 ID）、用户规则与内置规则的求值顺序，以及插件
// 上下文适配器的转发与缺失上下文报错。
package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// fakeProfileStore 为内存 KV 存储：键不存在时与宿主一致地返回零值存储值而非错误。
type fakeProfileStore struct {
	// values 为已写入的键值对。
	values map[string]string
	// getErr 非空时读取一律失败，用于覆盖读失败分支。
	getErr error
	// setErr 非空时写入一律失败，用于覆盖写失败分支。
	setErr error
}

// newFakeProfileStore 构造空的内存存储。
func newFakeProfileStore() *fakeProfileStore {
	return &fakeProfileStore{values: make(map[string]string)}
}

// GetValue 读取键对应明文；未写入的键返回零值存储值。
func (s *fakeProfileStore) GetValue(key string) (*sdkdto.StorageValue, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	value, ok := s.values[key]
	if !ok {
		return &sdkdto.StorageValue{}, nil
	}
	return &sdkdto.StorageValue{Value: value, SchemaVersion: CurrentSchemaVersion}, nil
}

// SetValue 覆写键对应明文。
func (s *fakeProfileStore) SetValue(key string, value string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.values[key] = value
	return nil
}

// ctxStub 为插件上下文桩：嵌入 PluginContext 仅实现 KV 两项，
// 未实现的方法一旦被调用即 panic 暴露测试缺口。
type ctxStub struct {
	sdkdto.PluginContext
	store *fakeProfileStore
}

// GetValue 转发到内存存储。
func (c ctxStub) GetValue(key string) (*sdkdto.StorageValue, error) {
	return c.store.GetValue(key)
}

// SetValue 转发到内存存储。
func (c ctxStub) SetValue(key string, value string) error {
	return c.store.SetValue(key, value)
}

// testRule 构造一条可序列化的规则，用于存取往返与顺序断言。
func testRule(t *testing.T, id string, enabled bool) RuleEntry {
	t.Helper()
	pattern, err := NewRegexPattern(string(TargetFileName), "^"+id+`\.txt$`, nil)
	if err != nil {
		t.Fatalf("构造测试规则 %q 的模式失败: %v", id, err)
	}
	return RuleEntry{
		ID:          id,
		Description: "测试规则 " + id,
		Source:      RuleSourceManual,
		Enabled:     enabled,
		Pattern:     pattern,
		Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromParentDir}},
	}
}

// marshalForCompare 把档案序列化为字符串，供往返等价比较。
func marshalForCompare(t *testing.T, p *RuleProfile) string {
	t.Helper()
	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("档案序列化失败: %v", err)
	}
	return string(data)
}

// TestProfileStoreRoundTrip 存读往返等价：写出的档案经同一存储取回后 JSON 完全一致，
// 且落在全库单份档案键上。
func TestProfileStoreRoundTrip(t *testing.T) {
	store := newFakeProfileStore()
	rules := []RuleEntry{testRule(t, "user-round-trip", true)}
	saved := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: rules}

	if err := SaveProfile(store, saved); err != nil {
		t.Fatalf("保存档案失败: %v", err)
	}
	if _, ok := store.values[RuleProfileKVKey]; !ok {
		t.Fatalf("档案应写入键 %q，实际键集合: %v", RuleProfileKVKey, store.values)
	}

	loaded, errs := LoadProfile(store)
	if len(errs) != 0 {
		t.Fatalf("载入档案不应报错，实际: %v", errs)
	}
	if loaded == nil {
		t.Fatalf("载入档案不应为 nil")
	}
	if loaded.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("结构版本应为 %d，实际 %d", CurrentSchemaVersion, loaded.SchemaVersion)
	}
	if len(loaded.Rules) != len(saved.Rules) {
		t.Fatalf("规则条数应为 %d，实际 %d", len(saved.Rules), len(loaded.Rules))
	}
	if got, want := marshalForCompare(t, loaded), marshalForCompare(t, saved); got != want {
		t.Fatalf("往返档案不一致\n取回: %s\n原始: %s", got, want)
	}
}

// TestSaveProfileNilWritesEmpty 传入 nil 档案时按空档案落库，取回后规则列表为空。
func TestSaveProfileNilWritesEmpty(t *testing.T) {
	store := newFakeProfileStore()
	if err := SaveProfile(store, nil); err != nil {
		t.Fatalf("保存空档案失败: %v", err)
	}
	loaded, errs := LoadProfile(store)
	if len(errs) != 0 {
		t.Fatalf("载入空档案不应报错，实际: %v", errs)
	}
	if loaded == nil || loaded.SchemaVersion != CurrentSchemaVersion || len(loaded.Rules) != 0 {
		t.Fatalf("空档案应为当前版本且规则为空，实际: %+v", loaded)
	}
}

// TestSaveProfileStoreErrors 存储不可用或写入失败时保存返回错误。
func TestSaveProfileStoreErrors(t *testing.T) {
	if err := SaveProfile(nil, &RuleProfile{SchemaVersion: CurrentSchemaVersion}); err == nil {
		t.Fatalf("存储为 nil 时保存应报错")
	}
	store := newFakeProfileStore()
	store.setErr = errors.New("注入的写入失败")
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion}); err == nil {
		t.Fatalf("写入失败时保存应报错")
	} else if !strings.Contains(err.Error(), RuleProfileKVKey) {
		t.Fatalf("写入失败错误应携带档案键名，实际: %v", err)
	}
}

// TestLoadProfileEmpty 无档案时返回空档案且无错误：键缺失、存储值为 nil、
// 明文为空三种形态等价。
func TestLoadProfileEmpty(t *testing.T) {
	cases := []struct {
		name  string
		store ProfileStore
	}{
		{name: "键缺失", store: newFakeProfileStore()},
		{name: "明文为空", store: &fakeProfileStore{values: map[string]string{RuleProfileKVKey: ""}}},
		{name: "存储值为 nil", store: nilValueStore{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loaded, errs := LoadProfile(tc.store)
			if len(errs) != 0 {
				t.Fatalf("无档案不应报错，实际: %v", errs)
			}
			if loaded == nil {
				t.Fatalf("无档案应返回空档案而非 nil")
			}
			if loaded.SchemaVersion != CurrentSchemaVersion {
				t.Fatalf("空档案结构版本应为 %d，实际 %d", CurrentSchemaVersion, loaded.SchemaVersion)
			}
			if len(loaded.Rules) != 0 {
				t.Fatalf("空档案规则列表应为空，实际 %d 条", len(loaded.Rules))
			}
		})
	}
}

// nilValueStore 为恒返回 nil 存储值的存储，用于覆盖宿主返回空值的形态。
type nilValueStore struct{}

// GetValue 返回 nil 存储值与无错误。
func (nilValueStore) GetValue(string) (*sdkdto.StorageValue, error) { return nil, nil }

// SetValue 为占位实现，测试中不会被调用。
func (nilValueStore) SetValue(string, string) error { return nil }

// TestLoadProfileBrokenData JSON 损坏或未知历史版本时自动重置（拒载死锁收
// 口）：返回空档案与单条「已自动重置」知会警告、档案键被覆写为空 v4 档、原
// 文进入备份键；未来更高版本不中断：返回非 nil 档案（尽力解析）与单条版本
// 告警、不动存储。
func TestLoadProfileBrokenData(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		wantReset bool
	}{
		{name: "JSON 截断", value: `{"schemaVersion":1,"rules":[`, wantReset: true},
		{name: "非 JSON 文本", value: "不是 JSON", wantReset: true},
		{name: "未知历史版本", value: `{"schemaVersion":0,"rules":[]}`, wantReset: true},
		{name: "未来更高版本", value: `{"schemaVersion":99,"rules":[]}`, wantReset: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: tc.value}}
			loaded, errs := LoadProfile(store)
			if loaded == nil {
				t.Fatal("档案应可用（重置为空档或尽力解析），不中断整体")
			}
			if len(errs) != 1 {
				t.Fatalf("应返回单个错误（重置知会或版本告警），实际 %d 个: %v", len(errs), errs)
			}
			if tc.wantReset {
				// 损坏/历史版本：自动重置知会 + 存储双写（档案键空档、备份键原文）
				if !strings.Contains(errs[0].Error(), "已自动重置为空档案") {
					t.Fatalf("历史/损坏档案应返回重置知会，实际: %v", errs[0])
				}
				if backup, _ := store.GetValue(RuleProfileBackupKVKey); backup == nil || backup.Value != tc.value {
					t.Fatalf("原文应进入备份键，实际: %+v", backup)
				}
				if cur, _ := store.GetValue(RuleProfileKVKey); cur == nil || cur.Value == tc.value {
					t.Fatalf("档案键应被覆写为空档（不再保留原文），实际: %+v", cur)
				}
				return
			}
			// 未来更高版本：告警且不动存储
			if !strings.Contains(errs[0].Error(), "高于当前支持的版本") {
				t.Fatalf("未来更高版本应返回单条版本告警，实际: %v", errs[0])
			}
			if cur, _ := store.GetValue(RuleProfileKVKey); cur == nil || cur.Value != tc.value {
				t.Fatalf("未来版本档案不应被重置，实际: %+v", cur)
			}
		})
	}
}

// TestLoadProfileLegacyAutoReset v3 旧档自动重置闭环（用户实测缺陷锚：拒载提
// 示指引「重置规则档案」但无重置入口，重新识别也不触碰档案存储键——旧档用
// 户被永久锁死）。断言：一次 LoadProfile 后返回空 v4 档+知会警告（含停用原
// 因）、KV 档案键已是 v4 空档、旧档原文在备份键；**二次 LoadProfile 干净**
// （无警告）；此后保存规则（UpdateProfile 链）畅通。
func TestLoadProfileLegacyAutoReset(t *testing.T) {
	v3 := `{"schemaVersion":3,"rules":[{"id":"old-expr","description":"v3 表达式规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^x\\.jpg$","captures":[]},` +
		`"extractions":[{"dimension":"title","from":"constant","constant":"旧值"}],"synthesis":{}}]}`
	store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: v3}}

	first, errs := LoadProfile(store)
	if first == nil {
		t.Fatalf("旧档应自动重置返回空档案，实际 nil（errs=%v）", errs)
	}
	if first.SchemaVersion != CurrentSchemaVersion || len(first.Rules) != 0 {
		t.Fatalf("重置应返回空 v4 档案，实际: 版本 %d、规则 %d 条", first.SchemaVersion, len(first.Rules))
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "已自动重置为空档案") || !strings.Contains(errs[0].Error(), "已停用") {
		t.Fatalf("应返回单条含停用原因的重置知会，实际: %v", errs)
	}
	if backup, _ := store.GetValue(RuleProfileBackupKVKey); backup == nil || backup.Value != v3 {
		t.Fatalf("旧档原文应进入备份键，实际: %+v", backup)
	}
	cur, _ := store.GetValue(RuleProfileKVKey)
	if cur == nil || strings.Contains(cur.Value, "old-expr") {
		t.Fatalf("档案键应为空 v4 档（旧规则不再在场），实际: %s", cur.Value)
	}

	second, errs2 := LoadProfile(store)
	if len(errs2) != 0 {
		t.Fatalf("重置后二次载入应无警告，实际: %v", errs2)
	}
	if second == nil || second.SchemaVersion != CurrentSchemaVersion || len(second.Rules) != 0 {
		t.Fatalf("二次载入应为空 v4 档案，实际: %+v", second)
	}
}

// TestLoadProfileReadError 读取失败时返回 nil 档案与错误。
func TestLoadProfileReadError(t *testing.T) {
	store := newFakeProfileStore()
	store.getErr = errors.New("注入的读取失败")
	loaded, errs := LoadProfile(store)
	if loaded != nil {
		t.Fatalf("读取失败时档案应为 nil，实际: %+v", loaded)
	}
	if len(errs) != 1 {
		t.Fatalf("读取失败时应返回单个错误，实际 %d 个: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), RuleProfileKVKey) {
		t.Fatalf("读取失败错误应携带档案键名，实际: %v", errs[0])
	}
}

// TestLoadProfileSkipsBrokenRule 坏规则被跳过且错误携带其规则 ID，其余规则照常载入。
func TestLoadProfileSkipsBrokenRule(t *testing.T) {
	value := fmt.Sprintf(`{"schemaVersion":%d,"rules":[`, CurrentSchemaVersion) +
		`{"id":"bad-regex","description":"正则非法","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"["},"extractions":[],"synthesis":{"dimPolicies":[],"roles":[]}},` +
		`{"id":"bad-missing-pattern","description":"缺模式","source":"manual","enabled":true,` +
		`"extractions":[],"synthesis":{"dimPolicies":[],"roles":[]}},` +
		`{"id":"good-rule","description":"完好规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^good$","captures":[]},"extractions":[],"synthesis":{"dimPolicies":[],"roles":[]}}` +
		`]}`
	store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: value}}

	loaded, errs := LoadProfile(store)
	if loaded == nil {
		t.Fatalf("坏规则不应导致整体载入失败")
	}
	if len(loaded.Rules) != 1 || loaded.Rules[0].ID != "good-rule" {
		t.Fatalf("应仅载入完好规则 good-rule，实际: %+v", loaded.Rules)
	}
	if len(errs) != 2 {
		t.Fatalf("应为两条坏规则各返回一个错误，实际 %d 个: %v", len(errs), errs)
	}
	joined := make([]string, 0, len(errs))
	for _, err := range errs {
		joined = append(joined, err.Error())
	}
	all := strings.Join(joined, "\n")
	for _, id := range []string{"bad-regex", "bad-missing-pattern"} {
		if !strings.Contains(all, id) {
			t.Fatalf("错误应携带规则 ID %q，实际: %s", id, all)
		}
	}
}

// TestEffectiveRulesOrder 求值顺序（v4 内置退休后）：档案内启用规则按档案序
// 出现，禁用规则不出现；序列收敛为纯用户规则。
func TestEffectiveRulesOrder(t *testing.T) {
	user := &RuleProfile{
		SchemaVersion: CurrentSchemaVersion,
		Rules: []RuleEntry{
			testRule(t, "user-on", true),
			testRule(t, "user-off", false),
			testRule(t, "user-on-later", true),
		},
	}

	effective := EffectiveRules(user)
	if len(effective) != 2 {
		t.Fatalf("求值序列应为 2 条用户启用规则，实际 %d 条", len(effective))
	}
	if effective[0].ID != "user-on" || effective[1].ID != "user-on-later" {
		t.Fatalf("用户规则应保持档案内顺序，实际: %q, %q", effective[0].ID, effective[1].ID)
	}
}

// TestEffectiveRulesNilUser 用户档案缺失时返回空序列（内置退休，无兜底规则）。
func TestEffectiveRulesNilUser(t *testing.T) {
	if effective := EffectiveRules(nil); len(effective) != 0 {
		t.Fatalf("用户档案缺失时应返回空序列，实际 %d 条", len(effective))
	}
}

// TestParseProfileRejectsLegacyVersions 旧档拒载断言（v4 零迁移）：结构版本
// 1..3 的档案整体拒绝（nil 档案 + 可读错误），不降级不部分载入。
func TestParseProfileRejectsLegacyVersions(t *testing.T) {
	legacy := []string{
		`{"schemaVersion":1,"rules":[]}`,
		`{"schemaVersion":2,"rules":[]}`,
		`{"schemaVersion":3,"rules":[{"id":"v3-rule","description":"d","source":"manual","enabled":true,` +
			`"pattern":{"kind":"regex","target":"filename","expr":"^a$","captures":[]},"extractions":[]}]}`,
	}
	for _, data := range legacy {
		profile, errs := ParseProfile([]byte(data))
		if profile != nil {
			t.Fatalf("旧档 %s 应整体拒载（nil 档案），实际 %+v", data, profile)
		}
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), "已停用") {
			t.Fatalf("旧档 %s 应回「已停用」错误，实际 %v", data, errs)
		}
	}
}

// TestContextProfileStoreAdapter 适配器把插件上下文的 KV 读写原样转发：
// 经适配器存读往返等价；上下文缺失时读写均报错。
func TestContextProfileStoreAdapter(t *testing.T) {
	backend := newFakeProfileStore()
	store := NewContextProfileStore(ctxStub{store: backend})
	if store == nil {
		t.Fatalf("适配器不应为 nil")
	}
	saved := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{testRule(t, "user-adapter", true)}}
	if err := SaveProfile(store, saved); err != nil {
		t.Fatalf("经适配器保存失败: %v", err)
	}
	loaded, errs := LoadProfile(store)
	if len(errs) != 0 {
		t.Fatalf("经适配器载入不应报错，实际: %v", errs)
	}
	if got, want := marshalForCompare(t, loaded), marshalForCompare(t, saved); got != want {
		t.Fatalf("经适配器往返档案不一致\n取回: %s\n原始: %s", got, want)
	}

	broken := NewContextProfileStore(nil)
	if _, err := broken.GetValue(RuleProfileKVKey); err == nil {
		t.Fatalf("上下文缺失时读取应报错")
	}
	if err := broken.SetValue(RuleProfileKVKey, "{}"); err == nil {
		t.Fatalf("上下文缺失时写入应报错")
	}
}

// —— 旧档案剥壳字段（stripPrefix / stripSuffix）的存量迁移 ——

// legacyStripProfileJSON 构造一条 v4 档案原文：目录行模板 + 捕获动作携带旧
// 剥壳前后缀，供迁移器断言（以 JSON 原文承载旧形态，不依赖已删除的模型字段）。
func legacyStripProfileJSON(tpl, prefix, suffix, from string) string {
	return fmt.Sprintf(`{"schemaVersion":%d,"rules":[{"id":"legacy-strip","description":"旧剥壳规则","source":"manual","enabled":true,`+
		`"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":%q}}}],`+
		`"extractions":[{"dimension":"tag","from":%q,"captureName":"tag","stripPrefix":%q,"stripSuffix":%q}],`+
		`"synthesis":{"dimPolicies":[],"roles":[]}}]}`, CurrentSchemaVersion, tpl, from, prefix, suffix)
}

// TestProfileLegacyStripMigrationOnLoad 载入路径的存量剥壳迁移：裸单占位词模板
// +配对括号剥壳 → 行模板改写为「开括号{词}闭括号{任意}」并产出首组内名
//（todo#96 场景）；其余带剥壳组合 → 剥壳丢弃+描述注记；迁移前原文进入一次性
// 迁移备份键（二次载入不覆写）；载入不回写档案键（下次保存自然落新形态、
// strip 键不再序列化）。
func TestProfileLegacyStripMigrationOnLoad(t *testing.T) {
	t.Run("裸单占位词配对括号改写并产出首组内名", func(t *testing.T) {
		raw := legacyStripProfileJSON("{标签}", "[", "]", "capture")
		store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: raw}}
		profile, errs := LoadProfile(store)
		if profile == nil || len(errs) != 0 {
			t.Fatalf("可等价改写档案应干净载入: %v", errs)
		}
		if len(profile.Rules) != 1 {
			t.Fatalf("应载入 1 条规则，实际 %d", len(profile.Rules))
		}
		rule := &profile.Rules[0]
		if tpl := rule.Conditions[0].Condition.Format.Tpl; tpl != "[{标签}]{任意}" {
			t.Fatalf("行模板应改写为括号模板 [{标签}]{任意}，实际 %q", tpl)
		}
		if a := rule.Extractions[0]; a.legacyStripPrefix != "" || a.legacyStripSuffix != "" {
			t.Fatalf("改写后剥壳暂存应清零: %+v", a)
		}
		if !strings.Contains(rule.Description, "剥壳已迁移为字面括号模板") {
			t.Fatalf("描述应含迁移注记，实际 %q", rule.Description)
		}
		if cur, _ := store.GetValue(RuleProfileKVKey); cur == nil || cur.Value != raw {
			t.Fatalf("载入不应回写档案键（原文待下次保存落新形态）: %s", cur.Value)
		}
		backup, _ := store.GetValue(RuleProfileMigrationBackupKVKey)
		if backup == nil || backup.Value != raw {
			t.Fatalf("迁移前档案原文应进入一次性迁移备份键，实际 %+v", backup)
		}
		// 迁移产物求值：多组名目录产出首组内名（七尾ゆきじ）。
		dirName := "[七尾ゆきじ] 酔いが醒めるその前に (COMIC 阿吽 2019年2月号) [中国翻訳] [DL版]"
		in := &MatchInput{FileName: "x.jpg", DirNames: []string{dirName}, RelPath: dirName + "/x.jpg"}
		decision := runRefEval(t, []RuleEntry{*rule}, in, nil)
		if v, ok := declOf(decision, rule.ID, DimTag); !ok || v != "七尾ゆきじ" {
			t.Fatalf("迁移后规则应产出首组内名 七尾ゆきじ: %q %v", v, ok)
		}
	})

	t.Run("不可等价改写组合剥壳丢弃", func(t *testing.T) {
		cases := []struct {
			name, tpl, prefix, suffix, from string
		}{
			{name: "模板带字面", tpl: "[{标签}]", prefix: "[", suffix: "]", from: "capture"},
			{name: "剥壳非配对括号", tpl: "{标签}", prefix: "【", suffix: "]", from: "capture"},
			{name: "单侧剥壳", tpl: "{标签}", prefix: "[", suffix: "", from: "capture"},
			{name: "非捕获来源", tpl: "{标签}", prefix: "[", suffix: "]", from: "ancestorDir"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				raw := legacyStripProfileJSON(tc.tpl, tc.prefix, tc.suffix, tc.from)
				store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: raw}}
				profile, errs := LoadProfile(store)
				if profile == nil || len(errs) != 0 {
					t.Fatalf("丢弃路径档案应干净载入: %v", errs)
				}
				rule := &profile.Rules[0]
				if tpl := rule.Conditions[0].Condition.Format.Tpl; tpl != tc.tpl {
					t.Fatalf("不可等价改写时行模板应保持原样 %q，实际 %q", tc.tpl, tpl)
				}
				if a := rule.Extractions[0]; a.legacyStripPrefix != "" || a.legacyStripSuffix != "" {
					t.Fatalf("丢弃后剥壳暂存应清零: %+v", a)
				}
				if !strings.Contains(rule.Description, "剥壳配置已移除，请改用字面括号模板") {
					t.Fatalf("描述应含丢弃注记，实际 %q", rule.Description)
				}
				if strings.Contains(rule.Description, "剥壳已迁移为字面括号模板") {
					t.Fatalf("丢弃路径不应携带改写注记: %q", rule.Description)
				}
				if backup, _ := store.GetValue(RuleProfileMigrationBackupKVKey); backup == nil || backup.Value != raw {
					t.Fatalf("丢弃路径同样属迁移处置，原文应进迁移备份键，实际 %+v", backup)
				}
			})
		}
	})

	t.Run("一次性迁移备份不覆写", func(t *testing.T) {
		raw := legacyStripProfileJSON("{标签}", "[", "]", "capture")
		store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: raw}}
		if _, errs := LoadProfile(store); len(errs) != 0 {
			t.Fatalf("首次载入应干净: %v", errs)
		}
		// 备份键已有内容（上一轮迁移写入）时，二次载入不得覆写。
		store.values[RuleProfileMigrationBackupKVKey] = "哨兵：更早的迁移备份"
		if _, errs := LoadProfile(store); len(errs) != 0 {
			t.Fatalf("二次载入应干净: %v", errs)
		}
		if got := store.values[RuleProfileMigrationBackupKVKey]; got != "哨兵：更早的迁移备份" {
			t.Fatalf("一次性迁移备份键不得覆写，实际 %q", got)
		}
	})

	t.Run("迁移后保存不再落 strip 键", func(t *testing.T) {
		raw := legacyStripProfileJSON("{标签}", "[", "]", "capture")
		src := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: raw}}
		profile, errs := LoadProfile(src)
		if profile == nil || len(errs) != 0 {
			t.Fatalf("载入失败: %v", errs)
		}
		dst := newFakeProfileStore()
		if err := SaveProfile(dst, profile); err != nil {
			t.Fatalf("保存迁移后档案失败: %v", err)
		}
		saved := dst.values[RuleProfileKVKey]
		if strings.Contains(saved, `"stripPrefix"`) || strings.Contains(saved, `"stripSuffix"`) {
			t.Fatalf("迁移后档案不应再序列化 strip 键: %s", saved)
		}
		if !strings.Contains(saved, "[{标签}]{任意}") {
			t.Fatalf("保存产物应携带改写后的括号模板: %s", saved)
		}
		if !strings.Contains(saved, "剥壳已迁移为字面括号模板") {
			t.Fatalf("保存产物应携带迁移注记: %s", saved)
		}
	})
}

// TestProfileNoStripNoMigrationBackup 无旧剥壳字段的档案不触发迁移：不写迁移
// 备份键、描述零变化（直接绿：现状载入路径本就不产生该键，锚定防回归）。
func TestProfileNoStripNoMigrationBackup(t *testing.T) {
	rule := testRule(t, "clean-rule", true)
	profileJSON := marshalForCompare(t, &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{rule}})
	store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: profileJSON}}
	loaded, errs := LoadProfile(store)
	if len(errs) != 0 {
		t.Fatalf("干净档案载入不应报错: %v", errs)
	}
	if got, want := marshalForCompare(t, loaded), profileJSON; got != want {
		t.Fatalf("干净档案应零迁移、往返等价\n取回: %s\n原始: %s", got, want)
	}
	if v, ok := store.values[RuleProfileMigrationBackupKVKey]; ok && v != "" {
		t.Fatalf("无剥壳档案不应写迁移备份键，实际 %q", v)
	}
}
