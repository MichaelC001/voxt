package hans

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSimplify(t *testing.T) {
	for in, want := range map[string]string{
		"頭髮":       "头发",
		"乾淨":       "干净",
		"後來我們去了公園": "后来我们去了公园",
		"這裡":       "这里",
		"這裏":       "这里",
		"看著他":      "看着他",
		"他著急地等著":   "他着急地等着",
		"著名作家的名著":  "著名作家的名著",
		"效果顯著":     "效果显著",
		"土著居民":     "土著居民",
		"乾隆年間":     "乾隆年间",
		"乾坤":       "乾坤",
		"為什麼":      "为什么",
		"這麼":       "这么",
		"我們用 Swift 寫了一個應用程式": "我们用 Swift 写了一个应用程式",
		"開會時間是下午三點，記得帶電腦。":   "开会时间是下午三点，记得带电脑。",
		// Simplified, English, empty: unchanged.
		"这里的头发很干净，后来著名的作家看着它。": "这里的头发很干净，后来著名的作家看着它。",
		"Hello, world!":           "Hello, world!",
		"":                        "",
		"Mixed 中文 and English 繁體": "Mixed 中文 and English 繁体",
		"張・三":                     "张・三",
	} {
		if got := Simplify(in); got != want {
			t.Errorf("Simplify(%q) = %q, want %q", in, got, want)
		}
	}
}

// Japanese with kanji that the tables convert (東, 時, 間, 電, 話, 後,
// 髮…): a text with kana is never changed.
func TestJapaneseUnchanged(t *testing.T) {
	for _, s := range []string{
		"東京駅で電話を掛けました。",
		"会議の後で、時間があれば連絡してください。",
		"彼女の髪は長くて綺麗です。",
		"日本の芸術と國語の歴史について話しましょう。",
		"気をつけて帰ってね。",
		"アプリの設定画面を開いて、言語を選んでください。",
		"人々は東京で働いている。",
		"ｶﾀｶﾅ 東京",
	} {
		if !Japanese(s) {
			t.Errorf("Japanese(%q) = false", s)
		}
		if got := Simplify(s); got != s {
			t.Errorf("Simplify(%q) = %q: Japanese changed", s, got)
		}
	}
	for _, s := range []string{"東京", "張・三", "hello", "头发"} {
		if Japanese(s) {
			t.Errorf("Japanese(%q) = true", s)
		}
	}
}

// Idempotent: the values are a fixed point.
func TestIdempotent(t *testing.T) {
	once.Do(load)
	for k, v := range chars {
		if again := Simplify(v); again != v {
			t.Errorf("%c → %s → %s", k, v, again)
		}
	}
	for k, v := range phrases {
		if again := Simplify(v); again != v {
			t.Errorf("%s → %s → %s", k, v, again)
		}
		if got := Simplify(k); got != v {
			t.Errorf("Simplify(%s) = %s, want %s", k, got, v)
		}
	}
	if len(chars) < 3000 || len(phrases) < 400 {
		t.Errorf("%d characters, %d phrases", len(chars), len(phrases))
	}
	for r := range chars {
		if r < 0x2E80 {
			t.Errorf("%U below the CJK blocks: first skips it", r)
		}
	}
	for k := range phrases {
		if r, _ := utf8.DecodeRuneInString(k); r < 0x2E80 {
			t.Errorf("%s below the CJK blocks: first skips it", k)
		}
	}
}

// A stream converted as it grows: each text so far converted whole, the
// end is the conversion of the whole text, whatever the pieces.
func TestStreamPrefixes(t *testing.T) {
	full := "乾隆年間，他看著那些著名的名著，頭髮都白了。"
	want := Simplify(full)
	var acc strings.Builder
	for _, r := range full {
		acc.WriteRune(r)
		_ = Simplify(acc.String())
	}
	if got := Simplify(acc.String()); got != want || want != "乾隆年间，他看着那些著名的名著，头发都白了。" {
		t.Errorf("stream %q, whole %q", got, want)
	}
}

func TestUnchangedNoAlloc(t *testing.T) {
	s := "This is English, 这是简体中文。"
	if n := testing.AllocsPerRun(100, func() { _ = Simplify(s) }); n != 0 {
		t.Errorf("%v allocations for a text with nothing to convert", n)
	}
}

func BenchmarkSimplify(b *testing.B) {
	s := strings.Repeat("後來我們在這裡開會，看著時間過去，頭髮都白了。", 40)
	b.SetBytes(int64(len(s)))
	for b.Loop() {
		Simplify(s)
	}
}
