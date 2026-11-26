package themes

import (
	"testing"
)

const boldFontStyle = "bold"

// @covers AC-PLUGINS-THEMES-002.1
func TestConvertVSCodeJSONC(t *testing.T) {
	data := []byte(`{ // a real exported palette
 "name":"Example","type":"dark",
 "colors":{"editor.background":"#272822","editor.foreground":"#f8f8f2","unknown.id":"bad"},
 "tokenColors":[{"name":"Comments","scope":["comment","string"],"settings":{"foreground":"#a6e22e","fontStyle":"italic"}},],
 "semanticTokenColors":{"variable.readonly":{"foreground":"#f92672","bold":true}},
 }`)
	imported, err := ConvertVSCode(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if imported.Definition.UI["background"] != "#272822" || imported.Definition.Syntax["comment"].Foreground != "#a6e22e" || imported.Report.Ignored != 1 {
		t.Fatalf("conversion = %+v", imported)
	}
	if imported.Definition.Semantic["variable.readonly"].FontStyle != boldFontStyle {
		t.Fatal("semantic style lost")
	}
	again, err := ConvertVSCode(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if imported.ID != again.ID {
		t.Fatal("identity unstable")
	}
}

func TestConvertVSCodeRejectsInvalidInputs(t *testing.T) {
	for _, data := range []string{`{"type":"dark","include":"./other.json"}`, `{"type":"dark","tokenColors":"./rules.json"}`, `{"type":"hcBlack"}`, `{"colors":{}}`, `{"type":"dark","colors":{"editor.background":"url(bad)"}}`, `{"type":"dark","colors":{"editor.background":"#fff","editor.background":"#000"}}`} {
		if _, err := ConvertVSCode([]byte(data), ""); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
}

// @covers AC-PLUGINS-THEMES-002.1, AC-PLUGINS-THEMES-002.2
func TestConvertVSCodeUnscopedDefaults(t *testing.T) {
	for _, scope := range []string{"", `"scope":"",`, `"scope":"  ",`} {
		t.Run(scope, func(t *testing.T) {
			data := []byte(`{"type":"dark","tokenColors":[{` + scope + `"settings":{"foreground":"#eeeeee","background":"#222222","fontStyle":"italic"}},{"settings":{"foreground":"#ffffff"}},{"scope":"comment","settings":{"foreground":"#aaaaaa","fontStyle":"bold"}}]}`)
			imported, err := ConvertVSCode(data, "")
			if err != nil {
				t.Fatal(err)
			}
			d := imported.Definition
			if d.Editor["foreground"] != "#ffffff" || d.Editor["background"] != "#222222" {
				t.Fatalf("defaults lost: %+v", d.Editor)
			}
			if len(d.Textmate) != 1 || d.Syntax["comment"].FontStyle != boldFontStyle {
				t.Fatal("scoped rules changed")
			}
			if imported.Report.Ignored != 1 {
				t.Fatalf("unsupported default style not reported: %+v", imported.Report)
			}
			again, err := ConvertVSCode(data, "")
			if err != nil || again.ID != imported.ID {
				t.Fatal("unstable source identity")
			}
		})
	}
}

func TestConvertVSCodeExplicitColorsOverrideDefaults(t *testing.T) {
	imported, err := ConvertVSCode([]byte(`{"type":"dark","colors":{"editor.foreground":"#abcdef","editor.background":"#123456"},"tokenColors":[{"settings":{"foreground":"#ffffff","background":"#000000"}}]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if imported.Definition.Editor["foreground"] != "#abcdef" || imported.Definition.Editor["background"] != "#123456" {
		t.Fatalf("explicit colors lost: %+v", imported.Definition.Editor)
	}
}

func TestConvertVSCodeRejectsInvalidDefaults(t *testing.T) {
	for _, settings := range []string{`{"foreground":"url(bad)"}`, `{"background":"#ffffff00"}`, `{"fontStyle":"blink"}`, `{"foreground":42}`, `{"unknown":"#ffffff"}`, `null`, `{"foreground":""}`} {
		data := []byte(`{"type":"dark","colors":{"editor.foreground":"#ffffff","editor.background":"#000000"},"tokenColors":[{"settings":` + settings + `}]}`)
		if _, err := ConvertVSCode(data, ""); err == nil {
			t.Errorf("accepted invalid defaults: %s", settings)
		}
	}
	for _, scope := range []string{`null`, `[]`, `42`, `[""]`} {
		if _, err := ConvertVSCode([]byte(`{"type":"dark","tokenColors":[{"scope":`+scope+`,"settings":{"foreground":"#ffffff"}}]}`), ""); err == nil {
			t.Errorf("accepted invalid scope %s", scope)
		}
	}
}
