package compose

import (
	"strings"
)

// AskInput is a question about a saved history entry (the History
// details' follow-up questions; Voxt's saved-transcription follow-up).
type AskInput struct {
	MainLanguage string
	// The entry: its feature ("transcribe", "translate", "rewrite"),
	// when it was made (as the user reads it), its result, and for
	// Rewrite the selection and the spoken instruction ("" for none).
	Kind, Created            string
	Result, Source, Instruct string
	// Turns are the questions asked before and their answers, oldest
	// first; Question is the new one.
	Turns    []AskTurn
	Question string
}

// AskTurn is an earlier question and its answer.
type AskTurn struct{ Question, Answer string }

// Ask is the request of a follow-up question: the rules (the template
// ask) in the system prompt, the entry in the first user message with
// the first question, then the turns as the chat they were. As
// everywhere in the composer, the entry's text is never in the system
// prompt.
func Ask(in AskInput) []Message {
	set := TemplateSet(in.MainLanguage)
	main := in.MainLanguage
	if strings.TrimSpace(main) == "" {
		main = "en"
	}
	sys := Render(template(set, "ask"), map[string]string{"USER_MAIN_LANGUAGE": LanguageName(main, set)}, nil)
	msgs := []Message{{Role: "system", Content: sys}}
	var b strings.Builder
	b.WriteString(`<entry kind="` + in.Kind + `" created="` + in.Created + `">` + "\n")
	block := func(tag, text string) {
		if text = strings.TrimSpace(text); text != "" {
			b.WriteString("<" + tag + ">\n" + text + "\n</" + tag + ">\n")
		}
	}
	block("selection", in.Source)
	block("instruction", in.Instruct)
	block("result", in.Result)
	b.WriteString("</entry>\n\n")
	first := b.String()
	for _, t := range in.Turns {
		msgs = append(msgs, Message{Role: "user", Content: first + strings.TrimSpace(t.Question)},
			Message{Role: "assistant", Content: strings.TrimSpace(t.Answer)})
		first = ""
	}
	return append(msgs, Message{Role: "user", Content: first + strings.TrimSpace(in.Question)})
}
