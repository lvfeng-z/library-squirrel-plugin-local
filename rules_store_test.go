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

// TestLoadProfileBrokenData JSON 损坏或未知历史版本时返回 nil 档案与单个错误；
// 未来更高版本不中断：返回非 nil 档案（尽力解析）与单条版本告警。
func TestLoadProfileBrokenData(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantNil bool
	}{
		{name: "JSON 截断", value: `{"schemaVersion":1,"rules":[`, wantNil: true},
		{name: "非 JSON 文本", value: "不是 JSON", wantNil: true},
		{name: "未知历史版本", value: `{"schemaVersion":0,"rules":[]}`, wantNil: true},
		{name: "未来更高版本", value: `{"schemaVersion":99,"rules":[]}`, wantNil: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: tc.value}}
			loaded, errs := LoadProfile(store)
			if tc.wantNil {
				if loaded != nil {
					t.Fatalf("数据不可用时档案应为 nil，实际: %+v", loaded)
				}
				if len(errs) != 1 {
					t.Fatalf("数据不可用时应返回单个错误，实际 %d 个: %v", len(errs), errs)
				}
				return
			}
			// 未来更高版本：告警且不中断（空规则档案照常可用）
			if loaded == nil {
				t.Fatal("未来更高版本应尽力解析返回非 nil 档案，不中断整体")
			}
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "高于当前支持的版本") {
				t.Fatalf("未来更高版本应返回单条版本告警，实际: %v", errs)
			}
		})
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
