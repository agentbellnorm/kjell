package kjell_test

import (
	"os"
	"os/exec"
	"testing"

	kjell "github.com/agentbellnorm/kjell"
	"github.com/agentbellnorm/kjell/internal/classifier"
)

var benchCommands = []struct {
	name  string
	input string
}{
	{"trivial", "ls"},
	{"flags_and_args", "grep -r --include='*.go' TODO ./src"},
	{"pipeline", "cat /etc/hosts | grep localhost | sort -u | head -5"},
	{"recursive_nested", "sudo env VAR=1 bash -c 'find /tmp -name \"*.log\" -exec rm {} ;'"},
	{"complex_script", `git status && docker exec -it myapp bash -c 'curl -s http://localhost/health | jq .status' || echo "failed" | tee /tmp/result.log`},
}

func BenchmarkClassifyEndToEnd(b *testing.B) {
	db, err := kjell.LoadDatabase()
	if err != nil {
		b.Fatal(err)
	}
	c := classifier.New(db)

	for _, tc := range benchCommands {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := c.Classify(tc.input)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkClassifyAll(b *testing.B) {
	db, err := kjell.LoadDatabase()
	if err != nil {
		b.Fatal(err)
	}
	c := classifier.New(db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range benchCommands {
			_, _ = c.Classify(tc.input)
		}
	}
}

func BenchmarkBinarySize(b *testing.B) {
	b.Helper()

	tmpDir := b.TempDir()
	out := tmpDir + "/kjell"

	cmd := exec.Command("go", "build", "-o", out, "./cmd/kjell/")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		b.Fatal(err)
	}

	info, err := os.Stat(out)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportMetric(float64(info.Size())/(1024*1024), "MB")
	b.ReportMetric(float64(info.Size()), "bytes")
}
