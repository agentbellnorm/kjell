package pyanalyze

import "github.com/agentbellnorm/kjell/internal/database"

type dangerEntry struct {
	effect database.Classification
	reason string
}

// dangerousBuiltins lists builtin functions that are inherently dangerous.
var dangerousBuiltins = map[string]dangerEntry{
	"exec":       {database.Unknown, "executes arbitrary code"},
	"eval":       {database.Unknown, "evaluates arbitrary expression"},
	"compile":    {database.Unknown, "compiles code object"},
	"__import__": {database.Unknown, "dynamic import"},
}

// safeBuiltins lists builtin functions known to be safe (no side effects
// beyond returning a value or printing to stdout).
var safeBuiltins = map[string]bool{
	// Output
	"print": true,
	// Type constructors / conversions
	"str": true, "int": true, "float": true, "bool": true, "bytes": true,
	"bytearray": true, "complex": true, "list": true, "dict": true,
	"tuple": true, "set": true, "frozenset": true, "memoryview": true,
	// Iteration
	"range": true, "enumerate": true, "zip": true, "map": true,
	"filter": true, "reversed": true, "sorted": true, "iter": true, "next": true,
	// Math
	"abs": true, "round": true, "min": true, "max": true, "sum": true,
	"pow": true, "divmod": true,
	// String / repr
	"format": true, "repr": true, "ascii": true, "chr": true, "ord": true,
	"hex": true, "oct": true, "bin": true,
	// Inspection
	"type": true, "isinstance": true, "issubclass": true, "callable": true,
	"len": true, "hash": true, "id": true, "dir": true, "vars": true,
	"hasattr": true, "getattr": true,
	// Other safe builtins
	"any": true, "all": true, "super": true, "object": true, "slice": true,
	"property": true, "staticmethod": true, "classmethod": true,
	"input": true,
}

type moduleInfo struct {
	defaultEffect database.Classification
	functions     map[string]database.Classification
}

// moduleTable defines the classification of calls to known modules.
// Modules not in this table are treated as unknown.
var moduleTable = map[string]moduleInfo{
	// --- Safe modules: all calls are side-effect free ---
	"json":        {defaultEffect: database.Safe},
	"sys":         {defaultEffect: database.Safe},
	"re":          {defaultEffect: database.Safe},
	"math":        {defaultEffect: database.Safe},
	"cmath":       {defaultEffect: database.Safe},
	"datetime":    {defaultEffect: database.Safe},
	"time":        {defaultEffect: database.Safe},
	"calendar":    {defaultEffect: database.Safe},
	"collections": {defaultEffect: database.Safe},
	"itertools":   {defaultEffect: database.Safe},
	"functools":   {defaultEffect: database.Safe},
	"operator":    {defaultEffect: database.Safe},
	"string":      {defaultEffect: database.Safe},
	"textwrap":    {defaultEffect: database.Safe},
	"pprint":      {defaultEffect: database.Safe},
	"base64":      {defaultEffect: database.Safe},
	"hashlib":     {defaultEffect: database.Safe},
	"hmac":        {defaultEffect: database.Safe},
	"copy":        {defaultEffect: database.Safe},
	"typing":      {defaultEffect: database.Safe},
	"enum":        {defaultEffect: database.Safe},
	"abc":         {defaultEffect: database.Safe},
	"dataclasses": {defaultEffect: database.Safe},
	"decimal":     {defaultEffect: database.Safe},
	"fractions":   {defaultEffect: database.Safe},
	"statistics":  {defaultEffect: database.Safe},
	"random":      {defaultEffect: database.Safe},
	"struct":      {defaultEffect: database.Safe},
	"binascii":    {defaultEffect: database.Safe},
	"codecs":      {defaultEffect: database.Safe},
	"csv":         {defaultEffect: database.Safe},
	"io":          {defaultEffect: database.Safe},
	"contextlib":  {defaultEffect: database.Safe},
	"traceback":   {defaultEffect: database.Safe},
	"ast":         {defaultEffect: database.Safe},
	"difflib":     {defaultEffect: database.Safe},
	"uuid":        {defaultEffect: database.Safe},
	"ipaddress":   {defaultEffect: database.Safe},
	"numbers":     {defaultEffect: database.Safe},
	"array":       {defaultEffect: database.Safe},
	"bisect":      {defaultEffect: database.Safe},
	"heapq":       {defaultEffect: database.Safe},
	"pathlib":     {defaultEffect: database.Safe},
	"argparse":    {defaultEffect: database.Safe},
	"logging":     {defaultEffect: database.Safe},
	"warnings":    {defaultEffect: database.Safe},
	"glob":        {defaultEffect: database.Safe},
	"fnmatch":     {defaultEffect: database.Safe},
	"platform":    {defaultEffect: database.Safe},
	"locale":      {defaultEffect: database.Safe},
	"unicodedata": {defaultEffect: database.Safe},
	// --- Write modules: all calls have side effects ---
	"subprocess": {defaultEffect: database.Write},
	"shutil":     {defaultEffect: database.Write},

	// --- Mixed modules ---
	"os": {
		defaultEffect: database.Unknown,
		functions: map[string]database.Classification{
			// Safe: reading system/file info
			"getcwd":        database.Safe,
			"getenv":        database.Safe,
			"getpid":        database.Safe,
			"getppid":       database.Safe,
			"getuid":        database.Safe,
			"getgid":        database.Safe,
			"getlogin":      database.Safe,
			"uname":         database.Safe,
			"cpu_count":     database.Safe,
			"listdir":       database.Safe,
			"scandir":       database.Safe,
			"walk":          database.Safe,
			"fspath":        database.Safe,
			"stat":          database.Safe,
			"lstat":         database.Safe,
			"access":        database.Safe,
			"get_exec_path": database.Safe,
			"urandom":       database.Safe,
			"strerror":      database.Safe,
			"isatty":        database.Safe,
			// Write: file operations
			"remove":     database.Write,
			"unlink":     database.Write,
			"rmdir":      database.Write,
			"removedirs": database.Write,
			"rename":     database.Write,
			"renames":    database.Write,
			"replace":    database.Write,
			"truncate":   database.Write,
			"mkdir":      database.Write,
			"makedirs":   database.Write,
			"chmod":      database.Write,
			"chown":      database.Write,
			"link":       database.Write,
			"symlink":    database.Write,
			// Write: process execution
			"system":  database.Write,
			"popen":   database.Write,
			"execl":   database.Write,
			"execle":  database.Write,
			"execlp":  database.Write,
			"execv":   database.Write,
			"execve":  database.Write,
			"execvp":  database.Write,
			"execvpe": database.Write,
			"spawnl":  database.Write,
			"spawnle": database.Write,
			"spawnlp": database.Write,
			"spawnv":  database.Write,
			"spawnve": database.Write,
			"spawnvp": database.Write,
		},
	},
}
