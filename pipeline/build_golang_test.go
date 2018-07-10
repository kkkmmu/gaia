package pipeline

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gaia-pipeline/gaia"
	hclog "github.com/hashicorp/go-hclog"
)

var killContext = false
var mockedOutput string
var mockedStatus = 0

func fakeExecCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	if killContext {
		c, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		ctx = c
	}
	cs := []string{"-test.run=TestExecCommandContextHelper", "--", name}
	cs = append(cs, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	arg := strings.Join(cs, ",")
	envArgs := os.Getenv("CMD_ARGS")
	if len(envArgs) != 0 {
		envArgs += ":" + arg
	} else {
		envArgs = arg
	}
	os.Setenv("CMD_ARGS", envArgs)
	es := strconv.Itoa(mockedStatus)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "EXIT_STATUS=" + es}
	return cmd
}

func TestExecCommandContextHelper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, mockedOutput)
	i, _ := strconv.Atoi(os.Getenv("EXIT_STATUS"))
	os.Exit(i)
}

func TestPrepareEnvironment(t *testing.T) {
	tmp := os.TempDir()
	gaia.Cfg = new(gaia.Config)
	gaia.Cfg.HomePath = tmp
	b := new(BuildPipelineGolang)
	p := new(gaia.CreatePipeline)
	err := b.PrepareEnvironment(p)
	if err != nil {
		t.Fatal("error was not expected when preparing environment: ", err)
	}
	var expectedDest = regexp.MustCompile(`^/.*/tmp/golang/src/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !expectedDest.MatchString(p.Pipeline.Repo.LocalDest) {
		t.Fatalf("expected destination is '%s', but was '%s'", expectedDest, p.Pipeline.Repo.LocalDest)
	}
}

func TestPrepareEnvironmentInvalidPathForMkdir(t *testing.T) {
	gaia.Cfg = new(gaia.Config)
	gaia.Cfg.HomePath = "/notexists"
	b := new(BuildPipelineGolang)
	p := new(gaia.CreatePipeline)
	err := b.PrepareEnvironment(p)
	if err == nil {
		t.Fatal("error was expected but none occurred")
	}
}

func TestExecuteBuild(t *testing.T) {
	execCommandContext = fakeExecCommandContext
	defer func() { execCommandContext = exec.CommandContext }()
	tmp := os.TempDir()
	gaia.Cfg = new(gaia.Config)
	gaia.Cfg.HomePath = tmp
	b := new(BuildPipelineGolang)
	p := new(gaia.CreatePipeline)
	err := b.ExecuteBuild(p)
	if err != nil {
		t.Fatal("error while running executebuild. none was expected")
	}
	expectedArgs := "-test.run=TestExecCommandContextHelper,--,/usr/local/bin/go,get,-d,./...:-test.run=TestExecCommandContextHelper,--,/usr/local/bin/go,build,-o,_"
	actualArgs := os.Getenv("CMD_ARGS")
	if expectedArgs != actualArgs {
		t.Fatalf("expected args '%s' actual args '%s'", expectedArgs, actualArgs)
	}
}

func TestExecuteBuildContextTimeout(t *testing.T) {
	execCommandContext = fakeExecCommandContext
	killContext = true
	defer func() { execCommandContext = exec.CommandContext }()
	defer func() { killContext = false }()
	tmp := os.TempDir()
	gaia.Cfg = new(gaia.Config)
	gaia.Cfg.HomePath = tmp
	// Initialize shared logger
	gaia.Cfg.Logger = hclog.New(&hclog.LoggerOptions{
		Level:  hclog.Trace,
		Output: hclog.DefaultOutput,
		Name:   "Gaia",
	})
	b := new(BuildPipelineGolang)
	p := new(gaia.CreatePipeline)
	err := b.ExecuteBuild(p)
	if err == nil {
		t.Fatal("no error found while expecting error.")
	}
	if err.Error() != "context deadline exceeded" {
		t.Fatal("context deadline should have been exceeded. was instead: ", err)
	}
}

func TestCopyBinary(t *testing.T) {
	tmp := os.TempDir()
	gaia.Cfg = new(gaia.Config)
	gaia.Cfg.HomePath = tmp
	// Initialize shared logger
	gaia.Cfg.Logger = hclog.New(&hclog.LoggerOptions{
		Level:  hclog.Trace,
		Output: hclog.DefaultOutput,
		Name:   "Gaia",
	})
	b := new(BuildPipelineGolang)
	p := new(gaia.CreatePipeline)
	p.Pipeline.Name = "main"
	p.Pipeline.Type = "go"
	p.Pipeline.Repo.LocalDest = tmp
	src := filepath.Join(tmp, appendTypeToName(p.Pipeline.Name, p.Pipeline.Type))
	dst := appendTypeToName(p.Pipeline.Name, p.Pipeline.Type)
	f, _ := os.Create(src)
	defer f.Close()
	defer os.Remove(dst)
	ioutil.WriteFile(src, []byte("testcontent"), 0666)
	err := b.CopyBinary(p)
	if err != nil {
		t.Fatal("error was not expected when copying binary: ", err)
	}
	content, err := ioutil.ReadFile(dst)
	if err != nil {
		t.Fatal("error encountered while reading destination file: ", err)
	}
	if string(content) != "testcontent" {
		t.Fatal("file content did not equal src content. was: ", string(content))
	}
}

func TestCopyBinarySrcDoesNotExist(t *testing.T) {
	tmp := os.TempDir()
	gaia.Cfg = new(gaia.Config)
	gaia.Cfg.HomePath = tmp
	// Initialize shared logger
	gaia.Cfg.Logger = hclog.New(&hclog.LoggerOptions{
		Level:  hclog.Trace,
		Output: hclog.DefaultOutput,
		Name:   "Gaia",
	})
	b := new(BuildPipelineGolang)
	p := new(gaia.CreatePipeline)
	p.Pipeline.Name = "main"
	p.Pipeline.Type = "go"
	p.Pipeline.Repo.LocalDest = "/noneexistent"
	err := b.CopyBinary(p)
	if err == nil {
		t.Fatal("error was expected when copying binary but none occurred ")
	}
	if err.Error() != "open /noneexistent/main_go: no such file or directory" {
		t.Fatal("a different error occurred then expected: ", err)
	}
}
