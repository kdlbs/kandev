package themes

// The v1 import mapping is deliberately narrower than the VS Code workbench API.
var vscodeColors = map[string][]string{
	"editor.background":  {"editor.background", "ui.background"},
	"editor.foreground":  {"editor.foreground", "ui.foreground"},
	"sideBar.background": {"ui.sidebar"}, "sideBar.foreground": {"ui.sidebar-foreground"}, "sideBar.border": {"ui.sidebar-border"},
	"panel.background": {"ui.card"}, "panel.border": {"ui.border"},
	"input.background": {"ui.input"}, "input.foreground": {"ui.foreground"}, "input.border": {"ui.border"},
	"button.background": {"ui.primary"}, "button.foreground": {"ui.primary-foreground"},
	"dropdown.background": {"ui.popover"}, "dropdown.foreground": {"ui.popover-foreground"}, "dropdown.border": {"ui.border"},
	"focusBorder": {"ui.ring"}, "foreground": {"ui.foreground"},
	"editorCursor.foreground": {"editor.cursor"}, "editor.selectionBackground": {"editor.selection"}, "editor.inactiveSelectionBackground": {"editor.inactive-selection"},
	"editor.lineHighlightBackground": {"editor.line-highlight"}, "editorLineNumber.foreground": {"editor.line-number"}, "editorLineNumber.activeForeground": {"editor.active-line-number"},
	"editorGutter.background": {"editor.gutter"}, "editorWidget.background": {"editor.widget-background"}, "editorWidget.foreground": {"editor.widget-foreground"}, "editorWidget.border": {"editor.widget-border"},
	"editorError.foreground": {"editor.error"}, "editorWarning.foreground": {"editor.warning"}, "editorInfo.foreground": {"editor.info"},
	"diffEditor.insertedLineBackground": {"editor.diff-added-line"}, "diffEditor.removedLineBackground": {"editor.diff-removed-line"},
	"diffEditor.insertedTextBackground": {"editor.diff-added-text"}, "diffEditor.removedTextBackground": {"editor.diff-removed-text"},
	"terminal.background": {"terminal.background"}, "terminal.foreground": {"terminal.foreground"}, "terminalCursor.foreground": {"terminal.cursor"}, "terminalCursor.background": {"terminal.cursor-accent"}, "terminal.selectionBackground": {"terminal.selection"},
	"terminal.ansiBlack": {"terminal.black"}, "terminal.ansiRed": {"terminal.red"}, "terminal.ansiGreen": {"terminal.green"}, "terminal.ansiYellow": {"terminal.yellow"}, "terminal.ansiBlue": {"terminal.blue"}, "terminal.ansiMagenta": {"terminal.magenta"}, "terminal.ansiCyan": {"terminal.cyan"}, "terminal.ansiWhite": {"terminal.white"},
	"terminal.ansiBrightBlack": {"terminal.brightBlack"}, "terminal.ansiBrightRed": {"terminal.brightRed"}, "terminal.ansiBrightGreen": {"terminal.brightGreen"}, "terminal.ansiBrightYellow": {"terminal.brightYellow"}, "terminal.ansiBrightBlue": {"terminal.brightBlue"}, "terminal.ansiBrightMagenta": {"terminal.brightMagenta"}, "terminal.ansiBrightCyan": {"terminal.brightCyan"}, "terminal.ansiBrightWhite": {"terminal.brightWhite"},
}

var scopeAliases = map[string]string{
	"comment": "comment", "keyword": "keyword", "storage": "keyword", "string": "string", "constant.numeric": "number", "constant.language.boolean": "boolean", "entity.name.type": "type", "support.type": "type", "entity.name.function": "function", "support.function": "function", "variable": "variable", "variable.parameter": "parameter", "variable.other.property": "property", "constant": "constant", "keyword.operator": "operator", "punctuation": "punctuation", "entity.name.tag": "tag", "entity.other.attribute-name": "attribute",
}
