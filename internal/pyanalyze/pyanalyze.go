// Package pyanalyze classifies Python code snippets by detecting function
// calls using tree-sitter and checking them against an allowlist of known-safe
// operations. Anything not explicitly recognized is classified as unknown;
// known-dangerous operations (os.remove, subprocess.run, etc.) are write.
//
// For calls like os.system("cmd") and subprocess.run(["cmd"]), the inner
// shell command is extracted and classified via a caller-provided callback,
// so that kjell's full shell classification pipeline applies recursively.
// Similarly, exec("python code") recurses back into this analyzer.
//
// This package is experimental — it depends on github.com/odvcencio/gotreesitter.
package pyanalyze

import (
	"strings"

	"github.com/agentbellnorm/kjell/internal/database"
	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Result holds the classification of a Python code snippet.
type Result struct {
	Classification database.Classification
	Reasons        []string
}

// ShellClassifyFunc classifies a shell command string. The classifier passes
// its own classify method here, breaking the circular dependency.
type ShellClassifyFunc func(cmd string) database.Classification

var lang *gotreesitter.Language

func init() {
	lang = grammars.PythonLanguage()
}

const maxPyDepth = 5

// Analyze parses a Python code string and classifies it. Every function call
// in the code is checked: known-safe calls are fine, known-write calls make
// the result write, and unrecognized calls make the result unknown.
//
// classifyShell is called for shell commands found inside Python (os.system,
// subprocess.run, etc.).
func Analyze(code string, classifyShell ShellClassifyFunc) Result {
	if classifyShell == nil {
		panic("pyanalyze: classifyShell must not be nil")
	}
	return analyzeAtDepth(code, classifyShell, 0)
}

func analyzeAtDepth(code string, classifyShell ShellClassifyFunc, depth int) Result {
	if depth >= maxPyDepth {
		return Result{Classification: database.Unknown, Reasons: []string{"max Python recursion depth"}}
	}

	parser := gotreesitter.NewParser(lang)
	src := []byte(code)
	tree, err := parser.Parse(src)
	if err != nil {
		return Result{Classification: database.Unknown, Reasons: []string{"failed to parse Python"}}
	}
	defer tree.Release()

	imp := collectImports(tree, src)

	ctx := &analyzeCtx{
		src:           src,
		imp:           imp,
		classifyShell: classifyShell,
		pyDepth:       depth,
	}

	classification := database.Safe
	var reasons []string

	gotreesitter.Walk(tree.RootNode(), func(node *gotreesitter.Node, d int) gotreesitter.WalkAction {
		if node.Type(lang) != "call" {
			return gotreesitter.WalkContinue
		}

		class, reason := ctx.classifyCall(node)
		if class != database.Safe {
			classification = worst(classification, class)
			if reason != "" {
				reasons = append(reasons, reason)
			}
		}

		return gotreesitter.WalkContinue
	})

	return Result{Classification: classification, Reasons: reasons}
}

// analyzeCtx holds state for a single analysis pass.
type analyzeCtx struct {
	src           []byte
	imp           *importMap
	classifyShell ShellClassifyFunc
	pyDepth       int
}

func (c *analyzeCtx) classifyCall(node *gotreesitter.Node) (database.Classification, string) {
	funcNode := node.ChildByFieldName("function", lang)
	if funcNode == nil {
		return database.Safe, ""
	}

	switch funcNode.Type(lang) {
	case "identifier":
		return c.classifyBareCall(funcNode.Text(c.src), node)
	case "attribute":
		return c.classifyAttrCall(node, funcNode)
	default:
		// Complex call expression (chained calls, subscripts, etc.) — can't determine target
		return database.Unknown, "unresolvable call expression"
	}
}

func (c *analyzeCtx) classifyBareCall(name string, callNode *gotreesitter.Node) (database.Classification, string) {
	// From-imported name: from os import remove; remove('foo')
	if imported, ok := c.imp.fromImports[name]; ok {
		// Try shell recursion for from-imported shell calls (from os import system; system("ls"))
		if class, reason, ok := c.tryShellRecursion(callNode, imported.module, imported.name); ok {
			return class, reason
		}
		class, reason := classifyModuleFunc(imported.module, imported.name)
		if reason == "" {
			reason = name + " (from " + imported.module + " import " + imported.name + ")"
		}
		return class, reason
	}

	// exec() / eval(): try to recursively analyze the Python code argument
	if name == "exec" || name == "eval" {
		if code, ok := extractFirstStringArg(callNode, c.src); ok {
			inner := analyzeAtDepth(code, c.classifyShell, c.pyDepth+1)
			return inner.Classification, name + " wraps Python: " + truncate(code, 60)
		}
		// Can't extract string → fall through to table (unknown)
	}

	// open() is mode-dependent
	if name == "open" {
		return classifyOpenCall(callNode, c.src), ""
	}

	// Known safe builtins
	if safeBuiltins[name] {
		return database.Safe, ""
	}

	// Known dangerous builtins
	if entry, ok := dangerousBuiltins[name]; ok {
		return entry.effect, name + ": " + entry.reason
	}

	// Unknown bare function call
	return database.Unknown, name + ": unknown function"
}

func (c *analyzeCtx) classifyAttrCall(callNode, funcNode *gotreesitter.Node) (database.Classification, string) {
	objNode := funcNode.ChildByFieldName("object", lang)
	attrNode := funcNode.ChildByFieldName("attribute", lang)
	if objNode == nil || attrNode == nil {
		return database.Safe, ""
	}

	// Only handle simple X.method() where X is an identifier
	if objNode.Type(lang) != "identifier" {
		return database.Safe, ""
	}

	objName := objNode.Text(c.src)
	method := attrNode.Text(c.src)

	// Resolve to module name
	moduleName := ""
	if actual, ok := c.imp.modules[objName]; ok {
		moduleName = actual
	} else if _, ok := moduleTable[objName]; ok {
		moduleName = objName
	}

	if moduleName != "" {
		// Try shell recursion for os.system("cmd"), subprocess.run(["cmd"]), etc.
		if class, reason, ok := c.tryShellRecursion(callNode, moduleName, method); ok {
			return class, reason
		}
		return classifyModuleFunc(moduleName, method)
	}

	// Not a module → method call on a local variable (data.keys(), etc.) → safe
	return database.Safe, ""
}

// shellRecursiveCalls maps module.function for calls where the first argument
// is a shell command (string or list of strings).
var shellRecursiveCalls = map[string]map[string]bool{
	"os":         {"system": true, "popen": true},
	"subprocess": {"run": true, "call": true, "check_call": true, "check_output": true, "Popen": true},
}

// tryShellRecursion attempts to extract a shell command from the first argument
// of a call and classify it via the shell classifier. Returns false if this call
// is not a shell-executing function or the argument can't be extracted.
func (c *analyzeCtx) tryShellRecursion(callNode *gotreesitter.Node, module, function string) (database.Classification, string, bool) {
	fns, ok := shellRecursiveCalls[module]
	if !ok || !fns[function] {
		return "", "", false
	}

	label := module + "." + function

	// Try string argument: os.system("ls -la")
	if cmd, ok := extractFirstStringArg(callNode, c.src); ok {
		class := c.classifyShell(cmd)
		return class, label + " wraps shell: " + truncate(cmd, 60), true
	}

	// Try list argument: subprocess.run(["ls", "-la"])
	if cmd, ok := extractFirstListArg(callNode, c.src); ok {
		class := c.classifyShell(cmd)
		return class, label + " wraps shell: " + truncate(cmd, 60), true
	}

	// Can't extract → fall through to table
	return "", "", false
}

// --- argument extraction ---

// extractFirstStringArg returns the content of the first argument if it's a string literal.
func extractFirstStringArg(callNode *gotreesitter.Node, src []byte) (string, bool) {
	argsNode := callNode.ChildByFieldName("arguments", lang)
	if argsNode == nil || argsNode.NamedChildCount() == 0 {
		return "", false
	}
	first := argsNode.NamedChild(0)
	if first.Type(lang) != "string" {
		return "", false
	}
	content := nodeStringContent(first, src)
	return content, content != ""
}

// extractFirstListArg returns a shell command string from a list literal first argument.
// e.g. ["ls", "-la"] → "ls -la"
func extractFirstListArg(callNode *gotreesitter.Node, src []byte) (string, bool) {
	argsNode := callNode.ChildByFieldName("arguments", lang)
	if argsNode == nil || argsNode.NamedChildCount() == 0 {
		return "", false
	}
	first := argsNode.NamedChild(0)
	if first.Type(lang) != "list" {
		return "", false
	}
	var parts []string
	for i := 0; i < int(first.NamedChildCount()); i++ {
		elem := first.NamedChild(i)
		if elem.Type(lang) != "string" {
			return "", false // non-string element → can't determine
		}
		s := nodeStringContent(elem, src)
		if s == "" {
			return "", false
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

// nodeStringContent extracts the text content of a tree-sitter string node,
// stripping quotes and string prefixes (r, b, u). Returns "" for f-strings
// or anything that can't be statically resolved.
func nodeStringContent(node *gotreesitter.Node, src []byte) string {
	s := node.Text(src)
	// Strip string prefixes: r, b, u (and uppercase)
	for len(s) > 0 {
		ch := s[0]
		if ch == 'r' || ch == 'b' || ch == 'u' || ch == 'R' || ch == 'B' || ch == 'U' {
			s = s[1:]
			continue
		}
		break
	}
	// f-strings can't be statically extracted
	if len(s) > 0 && (s[0] == 'f' || s[0] == 'F') {
		return ""
	}
	// Triple quotes
	for _, q := range []string{`"""`, `'''`} {
		if strings.HasPrefix(s, q) && strings.HasSuffix(s, q) && len(s) >= 6 {
			return s[3 : len(s)-3]
		}
	}
	// Single/double quotes
	if len(s) < 2 {
		return ""
	}
	quote := s[0]
	if quote != '\'' && quote != '"' {
		return ""
	}
	if s[len(s)-1] != quote {
		return ""
	}
	return s[1 : len(s)-1]
}

// --- module function lookup ---

// classifyModuleFunc looks up module.function in the module table.
func classifyModuleFunc(module, function string) (database.Classification, string) {
	info, ok := moduleTable[module]
	if !ok {
		return database.Unknown, module + "." + function + ": unknown module"
	}
	if class, ok := info.functions[function]; ok {
		reason := ""
		if class != database.Safe {
			reason = module + "." + function
		}
		return class, reason
	}
	reason := ""
	if info.defaultEffect != database.Safe {
		reason = module + "." + function
	}
	return info.defaultEffect, reason
}

// --- import tracking ---

type importedName struct {
	module string
	name   string
}

type importMap struct {
	modules     map[string]string       // local name -> module name
	fromImports map[string]importedName // local name -> {module, name}
}

func collectImports(tree *gotreesitter.Tree, src []byte) *importMap {
	imp := &importMap{
		modules:     make(map[string]string),
		fromImports: make(map[string]importedName),
	}

	gotreesitter.Walk(tree.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		nodeType := node.Type(lang)

		switch nodeType {
		case "import_statement":
			parseImportStatement(node, src, imp)
		case "import_from_statement":
			parseFromImportStatement(node, src, imp)
		}

		return gotreesitter.WalkContinue
	})

	return imp
}

func parseImportStatement(node *gotreesitter.Node, src []byte, imp *importMap) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type(lang) {
		case "dotted_name":
			name := child.Text(src)
			imp.modules[name] = name
		case "aliased_import":
			nameNode := child.ChildByFieldName("name", lang)
			aliasNode := child.ChildByFieldName("alias", lang)
			if nameNode != nil && aliasNode != nil {
				imp.modules[aliasNode.Text(src)] = nameNode.Text(src)
			} else if nameNode != nil {
				name := nameNode.Text(src)
				imp.modules[name] = name
			}
		}
	}
}

func parseFromImportStatement(node *gotreesitter.Node, src []byte, imp *importMap) {
	var moduleName string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type(lang) == "dotted_name" {
			moduleName = child.Text(src)
			break
		}
		if child.Type(lang) == "relative_import" {
			moduleName = child.Text(src)
			break
		}
	}
	if moduleName == "" {
		return
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type(lang)

		if childType == "dotted_name" && child.Text(src) == moduleName {
			continue
		}

		if childType == "dotted_name" {
			name := child.Text(src)
			imp.fromImports[name] = importedName{module: moduleName, name: name}
		}
		if childType == "aliased_import" {
			nameNode := child.ChildByFieldName("name", lang)
			aliasNode := child.ChildByFieldName("alias", lang)
			if nameNode != nil && aliasNode != nil {
				imp.fromImports[aliasNode.Text(src)] = importedName{module: moduleName, name: nameNode.Text(src)}
			} else if nameNode != nil {
				name := nameNode.Text(src)
				imp.fromImports[name] = importedName{module: moduleName, name: name}
			}
		}
	}
}

// --- open() handling ---

// classifyOpenCall uses the AST to extract the mode argument from an open() call.
// If the mode is a string literal, classify based on its content. If the mode
// can't be statically determined (variable, expression, f-string), return Unknown.
func classifyOpenCall(callNode *gotreesitter.Node, src []byte) database.Classification {
	argsNode := callNode.ChildByFieldName("arguments", lang)
	if argsNode == nil {
		return database.Safe // open() with no args — will error at runtime, but safe
	}

	// Check for mode= keyword argument
	for i := 0; i < int(argsNode.NamedChildCount()); i++ {
		child := argsNode.NamedChild(i)
		if child.Type(lang) == "keyword_argument" {
			nameNode := child.ChildByFieldName("name", lang)
			if nameNode != nil && nameNode.Text(src) == "mode" {
				valueNode := child.ChildByFieldName("value", lang)
				if valueNode == nil {
					return database.Unknown
				}
				return classifyModeNode(valueNode, src)
			}
		}
	}

	// Check second positional argument
	positional := 0
	for i := 0; i < int(argsNode.NamedChildCount()); i++ {
		child := argsNode.NamedChild(i)
		if child.Type(lang) == "keyword_argument" {
			continue
		}
		positional++
		if positional == 2 {
			return classifyModeNode(child, src)
		}
	}

	// No mode argument — default is "r" → safe
	return database.Safe
}

// classifyModeNode classifies an open() mode from an AST node.
// Returns the classification if the node is a string literal, Unknown otherwise.
func classifyModeNode(node *gotreesitter.Node, src []byte) database.Classification {
	if node.Type(lang) != "string" {
		return database.Unknown // variable, expression, f-string, etc.
	}
	mode := nodeStringContent(node, src)
	if mode == "" {
		return database.Unknown // f-string or unparseable
	}
	return classifyMode(mode)
}

func classifyMode(mode string) database.Classification {
	for _, ch := range mode {
		switch ch {
		case 'w', 'a', 'x', '+':
			return database.Write
		}
	}
	return database.Safe
}

// --- helpers ---

func worst(a, b database.Classification) database.Classification {
	rank := map[database.Classification]int{
		database.Safe:    0,
		database.Unknown: 1,
		database.Write:   2,
	}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
