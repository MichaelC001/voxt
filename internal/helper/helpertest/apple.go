package helpertest

// AppleSim: the helper's apple.* methods (docs/apple-models.md) with
// the Swift helper's rules and pretend models, for the fake helper
// process (fake_apple.go) and for tests in process (it is a Caller of
// those methods: internal/apple's tests, the app's fakeSupervisor).

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

// AppleSim is a pretend Apple Speech and Apple Intelligence. Its fields
// may be changed between calls (under no lock: set them before use, or
// with Set).
type AppleSim struct {
	mu sync.Mutex
	// Engine is the speech engine (proto.AppleEngine*), "" for none.
	Engine string
	// Locales are the speech locales' states (proto.Locale*); a locale
	// not listed is unsupported.
	Locales map[string]string
	// SpeechPerm is the Speech Recognition permission (proto.Perm*);
	// notDetermined becomes granted on the first apple.speech.
	SpeechPerm string
	// Intelligence is proto.IntelligenceAvailable or the reason it is
	// not (proto.AppleReason*); Languages its languages.
	Intelligence string
	Languages    []string
	// Words are what apple.speech hears: one more word in the partial
	// after each piece of audio, all of them in the final text (none
	// when no audio came).
	Words []string
	// Answer is apple.chat's answer to p (nil: "Apple: " and the last
	// user message); ChatErr fails every apple.chat.
	Answer  func(p proto.AppleChatParams) string
	ChatErr *proto.Error

	jobs map[string]*appleJob
	// Chats are the apple.chat params received, in order.
	Chats []proto.AppleChatParams
	// Speeches are the apple.speech params received, in order.
	Speeches []proto.AppleSpeechParams
}

type appleJob struct {
	pieces int
	bytes  int
	finish chan struct{}
	done   bool
	notify func(method string, params any)
	id     string
}

// NewAppleSim is a Mac on macOS 26 with Apple Intelligence on: English
// and Chinese installed for speech, Japanese downloadable.
func NewAppleSim() *AppleSim {
	return &AppleSim{
		Engine:       proto.AppleEngineAnalyzer,
		Locales:      map[string]string{"en-US": proto.LocaleInstalled, "zh-CN": proto.LocaleInstalled, "ja-JP": proto.LocaleAvailable},
		SpeechPerm:   proto.PermGranted,
		Intelligence: proto.IntelligenceAvailable,
		Languages:    []string{"en", "ja", "zh"},
		Words:        []string{"Hello", "from", "Apple", "Speech."},
		jobs:         map[string]*appleJob{},
	}
}

// Set changes the sim under its lock.
func (s *AppleSim) Set(fn func(s *AppleSim)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s)
}

// Status answers apple.status.
func (s *AppleSim) Status(p proto.AppleStatusParams) proto.AppleStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st proto.AppleStatus
	st.Speech.Engine = s.Engine
	if s.Engine == "" {
		st.Speech.Reason = proto.AppleReasonUnsupportedOS
	}
	st.Speech.Locales = []proto.AppleLocale{}
	for _, l := range p.Locales {
		st.Speech.Locales = append(st.Speech.Locales, proto.AppleLocale{Locale: l, Status: s.localeLocked(l)})
	}
	if s.Intelligence == proto.IntelligenceAvailable {
		st.Intelligence = proto.AppleIntelligenceStatus{Status: proto.IntelligenceAvailable, Languages: s.Languages}
	} else {
		st.Intelligence = proto.AppleIntelligenceStatus{Status: proto.IntelligenceUnavailable, Reason: s.Intelligence}
	}
	return st
}

func (s *AppleSim) localeLocked(l string) string {
	if v, ok := s.Locales[l]; ok && s.Engine != "" {
		return v
	}
	return proto.LocaleUnsupported
}

// Permission is the Speech Recognition permission now.
func (s *AppleSim) Permission() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SpeechPerm
}

// RequestPermission is permissions.request for speechRecognition: a
// permission not asked yet is granted.
func (s *AppleSim) RequestPermission() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SpeechPerm == proto.PermNotDetermined {
		s.SpeechPerm = proto.PermGranted
	}
	return s.SpeechPerm
}

// Install is the apple.install job: each available locale becomes
// installed, with progress at a half and at the end.
func (s *AppleSim) Install(ctx context.Context, id string, p proto.AppleInstallParams, notify func(string, any)) (proto.AppleInstallResult, error) {
	for _, f := range []float64{0.5, 1} {
		select {
		case <-ctx.Done():
			return proto.AppleInstallResult{}, proto.Errorf(proto.CodeCancelled, "The download was cancelled")
		case <-time.After(5 * time.Millisecond):
		}
		notify(proto.MethodAppleProgress, proto.AppleProgressParams{Job: id, Fraction: f})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res := proto.AppleInstallResult{Locales: []proto.AppleLocale{}}
	for _, l := range p.Locales {
		if s.localeLocked(l) == proto.LocaleAvailable {
			s.Locales[l] = proto.LocaleInstalled
		}
		res.Locales = append(res.Locales, proto.AppleLocale{Locale: l, Status: s.localeLocked(l)})
	}
	return res, nil
}

// Speech is the apple.speech job: it answers once Finish was called (or
// ctx cancelled: cancelled).
func (s *AppleSim) Speech(ctx context.Context, id string, p proto.AppleSpeechParams, notify func(string, any)) (proto.AppleSpeechResult, error) {
	s.mu.Lock()
	s.Speeches = append(s.Speeches, p)
	if s.Engine == "" {
		s.mu.Unlock()
		return proto.AppleSpeechResult{}, dataErr(proto.CodeUnsupported, map[string]string{"capability": proto.CapabilitySpeech}, "No speech recognition on this Mac")
	}
	if s.localeLocked(p.Locale) != proto.LocaleInstalled {
		s.mu.Unlock()
		return proto.AppleSpeechResult{}, dataErr(proto.CodeUnsupported, map[string]string{"capability": proto.CapabilityLocale}, "Speech recognition does not support %s on this Mac", p.Locale)
	}
	if s.SpeechPerm == proto.PermNotDetermined {
		s.SpeechPerm = proto.PermGranted
	}
	if s.SpeechPerm != proto.PermGranted {
		s.mu.Unlock()
		return proto.AppleSpeechResult{}, dataErr(proto.CodePermissionDenied, map[string]string{"permission": proto.PermissionSpeechRecognition}, "Speech recognition is not allowed")
	}
	j := &appleJob{finish: make(chan struct{}), notify: notify, id: id}
	s.jobs[id] = j
	engine := s.Engine
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.jobs, id)
		s.mu.Unlock()
	}()
	notify(proto.MethodAppleStarted, proto.AppleSpeechStartedParams{Job: id, Locale: p.Locale, Engine: engine})
	select {
	case <-ctx.Done():
		return proto.AppleSpeechResult{}, proto.Errorf(proto.CodeCancelled, "The recognition was cancelled")
	case <-j.finish:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	text := ""
	if j.pieces > 0 {
		text = strings.Join(s.Words, " ")
	}
	return proto.AppleSpeechResult{Text: text, Locale: p.Locale, Engine: engine, AudioMs: j.bytes / 32}, nil
}

// Audio is apple.speech.audio: one more word heard, as a partial.
func (s *AppleSim) Audio(p proto.AppleAudioParams) (proto.Empty, error) {
	s.mu.Lock()
	j := s.jobs[p.Job]
	if j == nil || j.done {
		s.mu.Unlock()
		return proto.Empty{}, proto.Errorf(proto.CodeBadRequest, "No apple.speech job %s", p.Job)
	}
	j.pieces++
	j.bytes += len(p.Data)
	n := min(j.pieces, len(s.Words))
	text := strings.Join(s.Words[:n], " ")
	s.mu.Unlock()
	if n > 0 {
		j.notify(proto.MethodApplePartial, proto.ApplePartialParams{Job: p.Job, Text: text})
	}
	return proto.Empty{}, nil
}

// Finish is apple.speech.finish.
func (s *AppleSim) Finish(p proto.AppleJobParams) (proto.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[p.Job]
	if j == nil || j.done {
		return proto.Empty{}, proto.Errorf(proto.CodeBadRequest, "No apple.speech job %s", p.Job)
	}
	j.done = true
	close(j.finish)
	return proto.Empty{}, nil
}

// Chat is the apple.chat job: the answer in two deltas.
func (s *AppleSim) Chat(ctx context.Context, id string, p proto.AppleChatParams, notify func(string, any)) (proto.AppleChatResult, error) {
	s.mu.Lock()
	s.Chats = append(s.Chats, p)
	reason, cerr, answer := s.Intelligence, s.ChatErr, s.Answer
	s.mu.Unlock()
	if reason != proto.IntelligenceAvailable {
		return proto.AppleChatResult{}, dataErr(proto.CodeUnsupported, map[string]string{"capability": proto.CapabilityIntelligence}, "Apple Intelligence is unavailable (%s)", reason)
	}
	if cerr != nil {
		return proto.AppleChatResult{}, cerr
	}
	text := ""
	if answer != nil {
		text = answer(p)
	} else {
		for _, m := range p.Messages {
			if m.Role == "user" {
				text = "Apple: " + m.Content
			}
		}
	}
	half := len(text) / 2
	for half > 0 && half < len(text) && text[half]&0xC0 == 0x80 { // a rune's start
		half++
	}
	for _, d := range []string{text[:half], text[half:]} {
		if ctx.Err() != nil {
			return proto.AppleChatResult{}, proto.Errorf(proto.CodeCancelled, "The answer was cancelled")
		}
		if d != "" {
			notify(proto.MethodAppleDelta, proto.AppleDeltaParams{Job: id, Text: d})
		}
	}
	return proto.AppleChatResult{Text: text, Finish: "stop"}, nil
}

func dataErr(code string, data any, format string, args ...any) *proto.Error {
	e := proto.Errorf(code, format, args...)
	e.Data, _ = json.Marshal(data)
	return e
}

// Call serves the apple.* methods as the helper's supervisor would
// (helper.Supervisor.Call): notify gets the job's notifications before
// Call returns; other methods are method_not_found.
func (s *AppleSim) Call(ctx context.Context, id, method string, params any, notify func(proto.Message)) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	send := func(m string, p any) {
		if notify != nil {
			n, _ := proto.NewNotification(m, p)
			notify(n)
		}
	}
	var v any
	switch method {
	case proto.MethodAppleStatus:
		var p proto.AppleStatusParams
		json.Unmarshal(raw, &p)
		v = s.Status(p)
	case proto.MethodAppleInstall:
		var p proto.AppleInstallParams
		json.Unmarshal(raw, &p)
		v, err = s.Install(ctx, id, p, send)
	case proto.MethodAppleSpeech:
		var p proto.AppleSpeechParams
		json.Unmarshal(raw, &p)
		v, err = s.Speech(ctx, id, p, send)
	case proto.MethodAppleSpeechAudio:
		var p proto.AppleAudioParams
		json.Unmarshal(raw, &p)
		v, err = s.Audio(p)
	case proto.MethodAppleSpeechFinish:
		var p proto.AppleJobParams
		json.Unmarshal(raw, &p)
		v, err = s.Finish(p)
	case proto.MethodAppleChat:
		var p proto.AppleChatParams
		json.Unmarshal(raw, &p)
		v, err = s.Chat(ctx, id, p, send)
	default:
		return nil, proto.Errorf(proto.CodeMethodNotFound, "Unknown method %s", method)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
