/*
The gomplate command

*/
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/env"
	"github.com/hairyhenderson/gomplate/v3/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile      string
	postRunInput *bytes.Buffer
)

const (
	defaultConfigName = ".gomplate"
	defaultConfigFile = ".gomplate.yaml"
)

func printVersion(name string) {
	fmt.Printf("%s version %s\n", name, version.Version)
}

// postRunExec - if templating succeeds, the command following a '--' will be executed
func postRunExec(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		name := args[0]
		args = args[1:]
		// nolint: gosec
		c := exec.Command(name, args...)
		if viper.GetBool("exec-pipe") {
			c.Stdin = postRunInput
		} else {
			c.Stdin = os.Stdin
		}
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout

		// make sure all signals are propagated
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs)
		go func() {
			// Pass signals to the sub-process
			sig := <-sigs
			if c.Process != nil {
				// nolint: gosec
				_ = c.Process.Signal(sig)
			}
		}()

		return c.Run()
	}
	return nil
}

// optionalExecArgs - implements cobra.PositionalArgs. Allows extra args following
// a '--', but not otherwise.
func optionalExecArgs(cmd *cobra.Command, args []string) error {
	if cmd.ArgsLenAtDash() == 0 {
		return nil
	}
	return cobra.NoArgs(cmd, args)
}

// process --include flags - these are analogous to specifying --exclude '*',
// then the inverse of the --include options.
func processIncludes(includes, excludes []string) []string {
	out := []string{}
	// if any --includes are set, we start by excluding everything
	if len(includes) > 0 {
		out = make([]string, 1+len(includes))
		out[0] = "*"
	}
	for i, include := range includes {
		// includes are just the opposite of an exclude
		out[i+1] = "!" + include
	}
	out = append(out, excludes...)
	return out
}

func newGomplateCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gomplate",
		Short: "Process text files with Go templates",
		// TODO: cmd.SetVersionTemplate(s string)
		// Version: version.Version,
		PreRunE: validateOpts,
		RunE: func(cmd *cobra.Command, args []string) error {
			if viper.GetBool("version") {
				printVersion(cmd.Name())
				return nil
			}
			verbose := viper.GetBool("verbose")
			if verbose {
				// nolint: errcheck
				fmt.Fprintf(os.Stderr, "%s version %s, build %s\nconfig is:\n%s\n\n",
					cmd.Name(), version.Version, version.GitCommit,
					viper.AllSettings())
			}

			conf := buildConfig(viper.GetViper())
			// support --include
			conf.ExcludeGlob = processIncludes(viper.GetStringSlice("include"), viper.GetStringSlice("exclude"))

			if viper.GetBool("exec-pipe") {
				postRunInput = &bytes.Buffer{}
				conf.Out = postRunInput
			}
			err := gomplate.RunTemplates(conf)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			if verbose {
				// nolint: errcheck
				fmt.Fprintf(os.Stderr, "rendered %d template(s) with %d error(s) in %v\n",
					gomplate.Metrics.TemplatesProcessed, gomplate.Metrics.Errors, gomplate.Metrics.TotalRenderDuration)
			}
			return err
		},
		PostRunE: postRunExec,
		Args:     optionalExecArgs,
	}
	return rootCmd
}

func buildConfig(v *viper.Viper) *gomplate.Config {
	g := &gomplate.Config{
		Input:       v.GetString("in"),
		InputFiles:  v.GetStringSlice("file"),
		InputDir:    v.GetString("input-dir"),
		ExcludeGlob: v.GetStringSlice("exclude"),
		OutputFiles: v.GetStringSlice("out"),
		OutputDir:   v.GetString("output-dir"),
		OutputMap:   v.GetString("output-map"),
		OutMode:     v.GetString("chmod"),

		DataSources:       v.GetStringSlice("datasource"),
		DataSourceHeaders: v.GetStringSlice("datasource-header"),
		Contexts:          v.GetStringSlice("context"),

		Plugins: v.GetStringSlice("plugin"),

		LDelim: v.GetString("left-delim"),
		RDelim: v.GetString("right-delim"),

		Templates: v.GetStringSlice("template"),
	}

	return g
}

func initFlags(command *cobra.Command) {
	command.Flags().SortFlags = false

	command.Flags().StringSliceP("datasource", "d", nil, "`datasource` in alias=URL form. Specify multiple times to add multiple sources.")
	command.Flags().StringSliceP("datasource-header", "H", nil, "HTTP `header` field in 'alias=Name: value' form to be provided on HTTP-based data sources. Multiples can be set.")

	command.Flags().StringSliceP("context", "c", nil, "pre-load a `datasource` into the context, in alias=URL form. Use the special alias `.` to set the root context.")

	command.Flags().StringSlice("plugin", nil, "plug in an external command as a function in name=path form. Can be specified multiple times")

	command.Flags().StringSliceP("file", "f", []string{"-"}, "Template `file` to process. Omit to use standard input, or use --in or --input-dir")
	command.Flags().StringP("in", "i", "", "Template `string` to process (alternative to --file and --input-dir)")
	command.Flags().String("input-dir", "", "`directory` which is examined recursively for templates (alternative to --file and --in)")

	command.Flags().StringSlice("exclude", []string{}, "glob of files to not parse")
	command.Flags().StringSlice("include", []string{}, "glob of files to parse")

	command.Flags().StringSliceP("out", "o", []string{"-"}, "output `file` name. Omit to use standard output.")
	command.Flags().StringSliceP("template", "t", []string{}, "Additional template file(s)")
	command.Flags().String("output-dir", ".", "`directory` to store the processed templates. Only used for --input-dir")
	command.Flags().String("output-map", "", "Template `string` to map the input file to an output path")
	command.Flags().String("chmod", "", "set the mode for output file(s). Omit to inherit from input file(s)")

	command.Flags().Bool("exec-pipe", false, "pipe the output to the post-run exec command")

	ldDefault := env.Getenv("GOMPLATE_LEFT_DELIM", "{{")
	rdDefault := env.Getenv("GOMPLATE_RIGHT_DELIM", "}}")
	command.Flags().String("left-delim", ldDefault, "override the default left-`delimiter` [$GOMPLATE_LEFT_DELIM]")
	command.Flags().String("right-delim", rdDefault, "override the default right-`delimiter` [$GOMPLATE_RIGHT_DELIM]")

	command.Flags().BoolP("verbose", "V", false, "output extra information about what gomplate is doing")

	command.Flags().BoolP("version", "v", false, "print the version")

	command.Flags().StringVar(&cfgFile, "config", defaultConfigFile, "config file (overridden by commandline flags)")

	viper.BindPFlags(command.Flags())

	cobra.OnInitialize(initConfig(command))
}

func initConfig(cmd *cobra.Command) func() {
	return func() {
		configRequired := false
		if cmd.Flags().Changed("config") {
			// Use config file from the flag.
			viper.SetConfigFile(cfgFile)
			configRequired = true
		} else {
			viper.AddConfigPath(".")
			viper.SetConfigName(defaultConfigName)
		}

		err := viper.ReadInConfig()
		if err != nil && configRequired {
			panic(err)
		}
		if err == nil && viper.GetBool("verbose") {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}

func main() {
	command := newGomplateCmd()
	initFlags(command)
	if err := command.Execute(); err != nil {
		// nolint: errcheck
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
