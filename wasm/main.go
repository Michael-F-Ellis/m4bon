//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/mellis/m4bon/midi"
	"github.com/mellis/m4bon/musicxml"
	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/render"
)

// --- Request types ---

type parseReq struct {
	DSL string `json:"dsl"`
}

type renderHTMLReq struct {
	DSL            string `json:"dsl"`
	ShowSubscripts bool   `json:"showSubscripts"`
	ShowComments   bool   `json:"showComments"`
	AsciiLeaps     bool   `json:"asciiLeaps"`
}

type smfReq struct {
	DSL        string  `json:"dsl"`
	BPM        float64 `json:"bpm"`
	Metronome  bool    `json:"metronome"`
	Roots      bool    `json:"roots"`
	Backbeats  bool    `json:"backbeats"`
}

type xmlReq struct {
	DSL string `json:"dsl"`
}

// --- Response helper ---

func ok(data any) string {
	resp, _ := json.Marshal(map[string]any{"ok": data})
	return string(resp)
}

func errMsg(msg string) string {
	resp, _ := json.Marshal(map[string]any{"err": msg})
	return string(resp)
}

// --- JSON event types for m4bonParse ---

type jsonFrac struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

type jsonPitch struct {
	Letter          string `json:"letter"`
	Accidental      int    `json:"accidental"`
	OctaveShift     int    `json:"octaveShift"`
	ExplicitNatural bool   `json:"explicitNatural,omitempty"`
}

type jsonEvent struct {
	Type            string       `json:"type"`
	Letter          string       `json:"letter,omitempty"`
	Accidental      int          `json:"accidental,omitempty"`
	EffAccidental   int          `json:"effAccidental,omitempty"`
	OctaveShift     int          `json:"octaveShift,omitempty"`
	Pitches         []jsonPitch  `json:"pitches,omitempty"`
	Midi            *int         `json:"midi"`
	Midis           []int        `json:"midis,omitempty"`
	Octave          *int         `json:"octave"`
	Octaves         []int        `json:"octaves,omitempty"`
	Duration        jsonFrac     `json:"duration"`
	Nominal         *jsonFrac    `json:"nominal,omitempty"`
	Split           bool         `json:"split"`
	Voice           int          `json:"voice"`
	GroupIdx        int          `json:"groupIdx"`
	NumSlots        int          `json:"numSlots"`
}

type jsonMeasure struct {
	TimeNum    int         `json:"timeNum"`
	TimeDen    int         `json:"timeDen"`
	Fifths     int         `json:"fifths"`
	IsPickup   bool        `json:"isPickup"`
	NumGroups  int         `json:"numGroups"`
	GroupSlots []int       `json:"groupSlots"`
	GroupMults []int       `json:"groupMults"`
	Events     []jsonEvent `json:"events"`
}

type parseOK struct {
	Measures []jsonMeasure `json:"measures"`
	KeyFifths int          `json:"keyFifths"`
	TimeNum   int          `json:"timeNum"`
	TimeDen   int          `json:"timeDen"`
}

func intPtr(v int) *int { return &v }

func eventToJSON(ev parser.Event) jsonEvent {
	je := jsonEvent{
		Type:     string(ev.Type),
		Duration: jsonFrac{Num: ev.Duration.Num, Den: ev.Duration.Den},
		Split:    ev.Split,
		Voice:    ev.Voice,
		GroupIdx: ev.GroupIdx,
		NumSlots: ev.NumSlots,
	}
	if ev.Nominal != nil {
		je.Nominal = &jsonFrac{Num: ev.Nominal.Num, Den: ev.Nominal.Den}
	}

	switch ev.Type {
	case parser.EventNote:
		je.Letter = ev.Letter
		je.Accidental = ev.Accidental
		je.EffAccidental = ev.EffAccidental
		je.OctaveShift = ev.OctaveShift
		je.Midi = intPtr(ev.Midi)
		je.Octave = intPtr(ev.ResolvedOctave - 1)
	case parser.EventChord:
		for _, p := range ev.Pitches {
			je.Pitches = append(je.Pitches, jsonPitch{
				Letter:          p.Letter,
				Accidental:      p.Accidental,
				OctaveShift:     p.OctaveShift,
				ExplicitNatural: p.ExplicitNatural,
			})
		}
		je.Midis = ev.Midis
		for _, o := range ev.ResolvedOctaves {
			je.Octaves = append(je.Octaves, o-1)
		}
	case parser.EventRest:
		// no extra fields
	}
	return je
}

// --- WASM exports ---

func parseWrapper(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errMsg("m4bonParse: expected 1 argument (JSON string)")
	}

	var req parseReq
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return errMsg("m4bonParse: invalid JSON: " + err.Error())
	}

	lines := parser.SanitizeDSL(req.DSL)
	if len(lines) == 0 {
		return errMsg("m4bonParse: empty DSL after sanitization")
	}

	result := parser.ParseDSL(lines)
	if result.Err != nil {
		return errMsg("m4bonParse: " + result.Err.Error())
	}

	measures := make([]jsonMeasure, len(result.Measures))
	for i, m := range result.Measures {
		events := make([]jsonEvent, len(m.Events))
		for j, ev := range m.Events {
			events[j] = eventToJSON(ev)
		}
		slots := m.GroupSlots
		if slots == nil {
			slots = []int{}
		}
		mults := m.GroupMults
		if mults == nil {
			mults = []int{}
		}
		measures[i] = jsonMeasure{
			TimeNum:    m.TimeNum,
			TimeDen:    m.TimeDen,
			Fifths:     m.Fifths,
			IsPickup:   m.IsPickup,
			NumGroups:  m.NumGroups,
			GroupSlots: slots,
			GroupMults: mults,
			Events:     events,
		}
	}

	return ok(parseOK{
		Measures:   measures,
		KeyFifths:  result.Key.Fifths,
		TimeNum:    result.TimeNum,
		TimeDen:    result.TimeDen,
	})
}

func renderHTMLWrapper(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errMsg("m4bonRenderHTML: expected 1 argument (JSON string)")
	}

	var req renderHTMLReq
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return errMsg("m4bonRenderHTML: invalid JSON: " + err.Error())
	}

	lines, comments := parser.SanitizeWithComments(req.DSL)
	if len(lines) == 0 {
		return errMsg("m4bonRenderHTML: empty DSL after sanitization")
	}

	result := parser.ParseDSLWithComments(lines, comments)
	if result.Err != nil {
		return errMsg("m4bonRenderHTML: " + result.Err.Error())
	}

	rows, maxCW, maxNW, maxLW := render.BuildRows(result.Measures, req.ShowSubscripts, req.ShowComments)
	html := render.FormatHTMLRows(rows, maxCW, maxNW, maxLW, req.AsciiLeaps)
	return ok(html)
}

func smfWrapper(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errMsg("m4bonGenerateSMF: expected 1 argument (JSON string)")
	}

	var req smfReq
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return errMsg("m4bonGenerateSMF: invalid JSON: " + err.Error())
	}

	if req.BPM <= 0 {
		req.BPM = 120
	}

	lines := parser.SanitizeDSL(req.DSL)
	if len(lines) == 0 {
		return errMsg("m4bonGenerateSMF: empty DSL after sanitization")
	}

	result := parser.ParseDSL(lines)
	if result.Err != nil {
		return errMsg("m4bonGenerateSMF: " + result.Err.Error())
	}

	el, err := midi.GenerateEventList(result.Measures, req.BPM, midi.SMFOptions{
		Metronome: req.Metronome,
		Roots:     req.Roots,
		Backbeats: req.Backbeats,
	})
	if err != nil {
		return errMsg("m4bonGenerateSMF: " + err.Error())
	}

	return ok(el)
}

func xmlWrapper(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errMsg("m4bonGenerateXML: expected 1 argument (JSON string)")
	}

	var req xmlReq
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return errMsg("m4bonGenerateXML: invalid JSON: " + err.Error())
	}

	lines := parser.SanitizeDSL(req.DSL)
	if len(lines) == 0 {
		return errMsg("m4bonGenerateXML: empty DSL after sanitization")
	}

	result := parser.ParseDSL(lines)
	if result.Err != nil {
		return errMsg("m4bonGenerateXML: " + result.Err.Error())
	}

	xml, err := musicxml.Generate(result.Measures, result.Key.Fifths)
	if err != nil {
		return errMsg("m4bonGenerateXML: " + err.Error())
	}

	return ok(xml)
}

func main() {
	js.Global().Set("m4bonParse", js.FuncOf(parseWrapper))
	js.Global().Set("m4bonRenderHTML", js.FuncOf(renderHTMLWrapper))
	js.Global().Set("m4bonGenerateSMF", js.FuncOf(smfWrapper))
	js.Global().Set("m4bonGenerateXML", js.FuncOf(xmlWrapper))
	js.Global().Set("_m4bonReady", js.ValueOf(true))
	<-make(chan struct{})
}
