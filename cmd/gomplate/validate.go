package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func isSet(cmd *cobra.Command, flag string) bool {
	return cmd.Flags().Changed(flag) || viper.InConfig(flag)
}

func notTogether(cmd *cobra.Command, flags ...string) error {
	found := ""
	for _, flag := range flags {
		if isSet(cmd, flag) {
			if found != "" {
				a := make([]string, len(flags))
				for i := range a {
					a[i] = "--" + flags[i]
				}
				return fmt.Errorf("only one of these flags is supported at a time: %s", strings.Join(a, ", "))
			}
			found = flag
		}
	}
	return nil
}

func mustTogether(cmd *cobra.Command, left, right string) error {
	if isSet(cmd, left) {
		if !isSet(cmd, right) {
			return fmt.Errorf("--%s must be set when --%s is set", right, left)
		}
	}

	return nil
}

func validateOpts(cmd *cobra.Command, args []string) (err error) {
	err = notTogether(cmd, "in", "file", "input-dir")
	if err == nil {
		err = notTogether(cmd, "out", "output-dir", "output-map", "exec-pipe")
	}

	f := viper.GetStringSlice("file")
	o := viper.GetStringSlice("out")
	if err == nil && len(f) != len(o) {
		err = fmt.Errorf("must provide same number of --out (%d) as --file (%d) options", len(o), len(f))
	}

	if err == nil && isSet(cmd, "exec-pipe") && len(args) == 0 {
		err = fmt.Errorf("--exec-pipe may only be used with a post-exec command after --")
	}

	if err == nil {
		err = mustTogether(cmd, "output-dir", "input-dir")
	}

	if err == nil {
		err = mustTogether(cmd, "output-map", "input-dir")
	}

	return err
}
