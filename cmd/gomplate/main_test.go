package main

import (
	"testing"

	"github.com/hairyhenderson/gomplate/v3"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestProcessIncludes(t *testing.T) {
	data := []struct {
		inc, exc, expected []string
	}{
		{nil, nil, []string{}},
		{[]string{}, []string{}, []string{}},
		{nil, []string{"*.foo"}, []string{"*.foo"}},
		{[]string{"*.bar"}, []string{"a*.bar"}, []string{"*", "!*.bar", "a*.bar"}},
		{[]string{"*.bar"}, nil, []string{"*", "!*.bar"}},
	}

	for _, d := range data {
		assert.EqualValues(t, d.expected, processIncludes(d.inc, d.exc))
	}
}

func TestBuildConfig(t *testing.T) {
	v := viper.New()
	c := buildConfig(v)
	expected := &gomplate.Config{}
	assert.Equal(t, expected, c)
}

func TestInitConfig(t *testing.T) {
	fs := afero.NewMemMapFs()
	viper.SetFs(fs)
	defer viper.Reset()
	cmd := &cobra.Command{}

	assert.NotPanics(t, func() {
		initConfig(cmd)()
	})

	cmd.Flags().String("config", "", "foo")

	assert.NotPanics(t, func() {
		initConfig(cmd)()
	})

	cmd.ParseFlags([]string{"--config", "config.file"})

	assert.Panics(t, func() {
		initConfig(cmd)()
	})

	f, err := fs.Create(defaultConfigName + ".yaml")
	assert.NoError(t, err)
	f.WriteString("")

	assert.NotPanics(t, func() {
		initConfig(cmd)()
	})
}
