package gc

import "io"

// Run and run keep the older tests short: they only look at the error.
func Run(out io.Writer, opts Options) error {
	_, err := RunResults(out, opts)
	return err
}

func run(out io.Writer, opts Options, d deps) error {
	_, err := runResults(out, opts, d)
	return err
}
