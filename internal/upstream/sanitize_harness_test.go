package upstream

import (
	"encoding/json"
	"strings"
	"testing"
)

// brand 竞品身份串（代码与测试共用的字面量）。
const brand = "DeepSeek Harness"

// CN 域（copilot.tencent.com）的渠道校验按 system 提示词里的竞品身份串拦整单
// （400 code=11-128 "Illegal API invocation from an unapproved channel"，2026-10-25
// 实测）。system 级指令里只改一个词，语义不变，但足以让整份 agent 提示词通过渠道校验。
func TestSanitizeBrandRewrittenInSystem(t *testing.T) {
	in := "You are an AI agent powered by " + brand + "."
	want := "You are an AI agent powered by DSH."
	if got := sanitizeSystemText(in); got != want {
		t.Errorf("sanitizeSystemText(%q) = %q, want %q", in, got, want)
	}
	in2 := brand + " checkout. " + brand + " Web GUI."
	want2 := "DSH checkout. DSH Web GUI."
	if got := sanitizeSystemText(in2); got != want2 {
		t.Errorf("sanitizeSystemText(%q) = %q, want %q", in2, got, want2)
	}
}

// 用户提示词与模型输出必须逐字保真：品牌词改写只允许发生在 system 消息里，
// 否则「原样输出以下文本」这类指令的回显会被换成 "DSH"，与用户的输入对不上。
func TestSanitizeBrandUntouchedOutsideSystem(t *testing.T) {
	cases := []string{
		"只原样输出以下文本\n" + brand + "\n" + brand + "\nDSH",
		"model answer mentions " + brand + " as a product name",
	}
	for _, in := range cases {
		if got := sanitizeText(in); got != in {
			t.Errorf("sanitizeText(%q) = %q, want unchanged", in, got)
		}
	}
}

// 消息级：同一请求里 system 的品牌词被改写，user / assistant 的逐字不动。
func TestSanitizeMessagesBrandSystemOnly(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "developer", "content": "powered by " + brand},
		map[string]any{"role": "system", "content": "powered by " + brand},
		map[string]any{"role": "user", "content": "echo " + brand},
		map[string]any{"role": "assistant", "content": "echo " + brand},
		map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{
			map[string]any{"id": "c1", "type": "function", "function": map[string]any{
				"name":      "Bash",
				"arguments": `{"command":"echo ` + brand + `"}`,
			}},
		}},
	}
	if !sanitizeMessages(msgs) {
		t.Fatal("sanitizeMessages 未报告任何改动（system 里的品牌词应被改写）")
	}
	get := func(i int) map[string]any { return msgs[i].(map[string]any) }
	for i, want := range []string{
		"powered by DSH", // developer：与 system 同级，改写
		"powered by DSH", // system：改写
		"echo " + brand,  // user：逐字不动
		"echo " + brand,  // assistant（模型输出）：逐字不动
	} {
		if got, _ := get(i)["content"].(string); got != want {
			t.Errorf("msgs[%d].content = %q, want %q", i, got, want)
		}
	}
	fn := get(4)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if args, _ := fn["arguments"].(string); !strings.Contains(args, brand) {
		t.Errorf("tool_calls arguments 品牌词被改写: %q", args)
	}
}

// 出站边界：PrepareBodyOpt 后 system 无品牌串、user/assistant 原样保留。
func TestPrepareBodyBrandSystemOnly(t *testing.T) {
	body := []byte(`{"model":"glm-5.2","messages":[` +
		`{"role":"system","content":"You are an agent powered by ` + brand + `."},` +
		`{"role":"user","content":"原样输出：` + brand + `"},` +
		`{"role":"assistant","content":"` + brand + `"}]}`)
	out := PrepareBodyOpt(body, true)
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	msgs := obj["messages"].([]any)
	sys, _ := msgs[0].(map[string]any)["content"].(string)
	if strings.Contains(sys, brand) || !strings.Contains(sys, "powered by DSH.") {
		t.Errorf("system 品牌串未改写: %q", sys)
	}
	user, _ := msgs[1].(map[string]any)["content"].(string)
	if user != "原样输出："+brand {
		t.Errorf("user 消息被改写: %q", user)
	}
	asst, _ := msgs[2].(map[string]any)["content"].(string)
	if asst != brand {
		t.Errorf("assistant 消息被改写: %q", asst)
	}
}
