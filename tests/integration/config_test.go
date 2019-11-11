//+build integration

package integration

import (
	"bytes"
	"io/ioutil"

	. "gopkg.in/check.v1"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
	"gotest.tools/v3/icmd"
)

type ConfigSuite struct {
	tmpDir *fs.Dir
}

var _ = Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *C) {
	s.tmpDir = fs.NewDir(c, "gomplate-inttests",
		fs.WithDir("indir"),
		fs.WithDir("outdir"),
		fs.WithFile(".gomplate.yaml", "in: hello world\n"),
	)
}

func (s *ConfigSuite) writeFile(f, content string) {
	f = s.tmpDir.Join(f)
	err := ioutil.WriteFile(f, []byte(content), 0644)
	if err != nil {
		panic(err)
	}
}

func (s *ConfigSuite) writeConfig(content string) {
	s.writeFile(".gomplate.yaml", content)
}

func (s *ConfigSuite) TearDownTest(c *C) {
	s.tmpDir.Remove()
}

func (s *ConfigSuite) TestReadsFromConfigFile(c *C) {
	result := icmd.RunCmd(icmd.Command(GomplateBin), func(cmd *icmd.Cmd) {
		cmd.Dir = s.tmpDir.Path()
	})

	s.writeConfig("file: -\n")
	result = icmd.RunCmd(icmd.Command(GomplateBin), func(cmd *icmd.Cmd) {
		cmd.Dir = s.tmpDir.Path()
		cmd.Stdin = bytes.NewBufferString("hello world")
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "hello world"})

	s.writeConfig("file: in\n")
	s.writeFile("in", "blah blah")
	result = icmd.RunCmd(icmd.Command(GomplateBin), func(cmd *icmd.Cmd) {
		cmd.Dir = s.tmpDir.Path()
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "blah blah"})

	s.writeConfig(`file: in
datasource:
  - data=in.yaml
`)
	s.writeFile("in", `{{ (ds "data").value }}`)
	s.writeFile("in.yaml", `value: hello world`)
	result = icmd.RunCmd(icmd.Command(GomplateBin), func(cmd *icmd.Cmd) {
		cmd.Dir = s.tmpDir.Path()
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "hello world"})

	s.writeConfig(`input-dir: indir/
output-dir: outdir/
datasource:
  - data=in.yaml
`)
	s.writeFile("indir/file", `{{ (ds "data").value }}`)
	s.writeFile("in.yaml", `value: hello world`)
	result = icmd.RunCmd(icmd.Command(GomplateBin), func(cmd *icmd.Cmd) {
		cmd.Dir = s.tmpDir.Path()
	})
	b, err := ioutil.ReadFile(s.tmpDir.Join("outdir", "file"))
	assert.NilError(c, err)
	assert.Equal(c, "hello world", string(b))
	result.Assert(c, icmd.Expected{ExitCode: 0})
}
