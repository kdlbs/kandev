package lsp

const (
	rpcVersion = "2.0"
	jsonNull   = "null"

	methodExit        = "exit"
	methodInitialized = "initialized"
	methodShutdown    = "shutdown"
	methodProgress    = "$/progress"
	methodDidOpen     = "textDocument/didOpen"
	methodDidChange   = "textDocument/didChange"
	methodDidSave     = "textDocument/didSave"
	methodDidClose    = "textDocument/didClose"

	fieldTextDocument     = "textDocument"
	fieldURI              = "uri"
	fieldLanguageID       = "languageId"
	fieldVersion          = "version"
	fieldText             = "text"
	fieldContentChanges   = "contentChanges"
	fieldWorkspaceFolders = "workspaceFolders"
	fieldDynamicRegister  = "dynamicRegistration"

	languagePython     = "python"
	languageRust       = "rust"
	languageTypeScript = "typescript"
)
